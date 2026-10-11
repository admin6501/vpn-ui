package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/corebundle"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/web/controller"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"github.com/op/go-logging"
	"golang.org/x/net/proxy"
)

func TestNodeProcessHelper(t *testing.T) {
	if os.Getenv("VPNUI_TEST_AGENT_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	a, err := New(os.Getenv("VPNUI_TEST_AGENT_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, 7*time.Minute)
	defer cancel()
	if err = a.Run(ctx); err != nil && ctx.Err() == nil {
		t.Fatal(err)
	}
}
func availablePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func waitNode(t *testing.T, timeout time.Duration, description string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("timed out: " + description)
}

// Runs independent central/agent SQLite databases and two real Xray processes.
// The node has no inbound management socket. No host VPN/kernel package is installed.
func TestNodeRealDataPlane(t *testing.T) {
	if os.Getenv("RUN_NODE_INTEGRATION") != "1" {
		t.Skip("enable with RUN_NODE_INTEGRATION=1 after building native Xray assets")
	}
	logger.InitLogger(logging.ERROR)
	gin.SetMode(gin.ReleaseMode)
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	svc := &service.NodeService{}
	node, pair, err := svc.Create("Integration", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	port := availablePort(t)
	uuid := "11111111-2222-3333-4444-555555555555"
	email := "native-node@example.com"
	clients, _ := json.Marshal(map[string]any{"clients": []map[string]any{{"id": uuid, "email": email, "enable": true, "subId": "integration-sub", "totalGB": 10 * 1024 * 1024}}, "decryption": "none"})
	in := model.Inbound{NodeID: node.ID, UserId: 1, Tag: "node-integration", Protocol: model.VLESS, Port: port, Listen: "127.0.0.1", Enable: true, Settings: string(clients), StreamSettings: `{"network":"tcp","security":"none"}`}
	db := database.GetDB()
	if err = db.Create(&in).Error; err != nil {
		t.Fatal(err)
	}
	(&service.AccountService{}).MigrationAccounts()
	if err = db.Create(&xray.ClientTraffic{Email: email, InboundId: in.Id, Enable: true, Total: 10 * 1024 * 1024}).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	controller.RegisterNodeAgentRoutes(r.Group("/central"))
	var unreachable atomic.Bool
	central := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if unreachable.Load() {
			w.WriteHeader(503)
			return
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, req)
		if recorder.Code == 200 && strings.HasSuffix(req.URL.Path, "/sync") {
			var snap nodewire.Snapshot
			if e := json.Unmarshal(recorder.Body.Bytes(), &snap); e != nil {
				t.Error(e)
				w.WriteHeader(500)
				return
			}
			// The target is a loopback test server; stock routing deliberately blocks
			// private destinations. Permit them only in this synthetic fixture.
			snap.Template = strings.ReplaceAll(snap.Template, `"outboundTag": "blocked"`, `"outboundTag": "direct"`)
			snap.Template = strings.ReplaceAll(snap.Template, `"domainStrategy": "AsIs"`, `"domainStrategy": "AsIs", "ipsBlocked": []`)
			json.NewEncoder(w).Encode(snap)
			return
		}
		w.WriteHeader(recorder.Code)
		w.Write(recorder.Body.Bytes())
	}))
	defer central.Close()
	folder := t.TempDir()
	cfg := filepath.Join(folder, "agent.json")
	if err = Enroll(context.Background(), cfg, central.URL+"/central/", pair, ""); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(binary, "-test.run=^TestNodeProcessHelper$", "-test.v")
	child.Env = append(os.Environ(), "VPNUI_TEST_AGENT_HELPER=1", "VPNUI_TEST_AGENT_CONFIG="+cfg, "VPNUI_DB_FOLDER="+folder, "VPNUI_BIN_FOLDER="+filepath.Join(folder, "bin"), "VPNUI_NODE_AGENT=1")
	agentLog, err := os.Create(filepath.Join(folder, "agent.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer agentLog.Close()
	child.Stdout = agentLog
	child.Stderr = agentLog
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		child.Process.Signal(os.Interrupt)
		child.Wait()
		if t.Failed() {
			raw, _ := os.ReadFile(agentLog.Name())
			t.Log(string(raw))
			configRaw, _ := os.ReadFile(filepath.Join(folder, "bin", "config.json"))
			t.Log("synthetic worker config:", string(configRaw))
		}
	}()
	waitNode(t, 15*time.Second, "first outbound sync", func() bool { var n model.RemoteNode; db.First(&n, node.ID); return n.LastSeen > 0 })
	if _, err = os.Stat(filepath.Join(folder, "bin", corebundle.XrayBinaryName())); !os.IsNotExist(err) {
		t.Fatal("core extracted before operator request", err)
	}
	if err = svc.InstallCores(node.ID, []string{"xray"}); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 30*time.Second, "selected Xray starts on node", func() bool {
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
		if e == nil {
			c.Close()
			return true
		}
		return false
	})
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(bytes.Repeat([]byte("x"), 256*1024)) }))
	defer target.Close()
	clientBin, err := corebundle.ExtractXray(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	socksPort := availablePort(t)
	clientConf := fmt.Sprintf(`{"log":{"loglevel":"warning"},"inbounds":[{"listen":"127.0.0.1","port":%d,"protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"127.0.0.1","port":%d,"users":[{"id":"%s","encryption":"none"}]}]},"streamSettings":{"network":"tcp","security":"none"}}]}`, socksPort, port, uuid)
	clientPath := filepath.Join(folder, "client.json")
	os.WriteFile(clientPath, []byte(clientConf), 0600)
	client := exec.Command(clientBin, "run", "-c", clientPath)
	client.Stdout = agentLog
	client.Stderr = agentLog
	if err = client.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { client.Process.Kill(); client.Wait() }()
	socksAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPort))
	waitNode(t, 10*time.Second, "test SOCKS client", func() bool {
		c, e := net.DialTimeout("tcp", socksAddr, 200*time.Millisecond)
		if e == nil {
			c.Close()
			return true
		}
		return false
	})
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialer.(proxy.ContextDialer).DialContext(ctx, network, address)
	}}}
	response, err := httpClient.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || len(payload) != 256*1024 {
		t.Fatal("data plane failed", len(payload), err)
	}
	waitNode(t, 35*time.Second, "central quota receives actual node bytes", func() bool {
		var tr xray.ClientTraffic
		db.Where("email = ?", email).First(&tr)
		return tr.Down >= 256*1024
	})
	var before xray.ClientTraffic
	db.Where("email = ?", email).First(&before)
	var cursor model.NodeCounter
	db.Where("node_id = ? AND kind = ?", node.ID, "account").First(&cursor)
	if before.AllTime < 256*1024 || cursor.Down < 256*1024 {
		t.Fatal("real traffic not accounted", before, cursor)
	}
	listener := func() bool {
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
		if e == nil {
			c.Close()
			return true
		}
		return false
	}
	if err = svc.Control(node.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 15*time.Second, "stop closes VPN but preserves management", func() bool { return !listener() })
	if err = svc.Control(node.ID, "start"); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 15*time.Second, "start reopens node VPN", listener)
	if err = svc.Control(node.ID, "restart"); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 15*time.Second, "restart acknowledged", func() bool {
		var n model.RemoteNode
		db.First(&n, node.ID)
		var report map[string]any
		json.Unmarshal([]byte(n.CoreReport), &report)
		return report["controlGeneration"] == "3" && listener()
	})
	if err = svc.Configure(node.ID, nodewire.ConfigCommand{Core: "xray", Inspect: true}); err != nil {
		t.Fatal(err)
	}
	var current string
	waitNode(t, 15*time.Second, "read actual node configuration", func() bool {
		var n model.RemoteNode
		db.First(&n, node.ID)
		var report struct {
			Generation string `json:"configGeneration"`
			Targets    []struct {
				Current string `json:"current"`
			} `json:"configTargets"`
		}
		json.Unmarshal([]byte(n.CoreReport), &report)
		if report.Generation == "1" && len(report.Targets) > 0 {
			current = report.Targets[0].Current
			return current != ""
		}
		return false
	})
	invalid := strings.ReplaceAll(current, `"protocol": "freedom"`, `"protocol": "not-a-real-protocol"`)
	if invalid == current {
		var cfg map[string]any
		json.Unmarshal([]byte(current), &cfg)
		cfg["outbounds"] = []any{map[string]any{"protocol": "not-a-real-protocol", "tag": "direct"}}
		raw, _ := json.Marshal(cfg)
		invalid = string(raw)
	}
	if err = svc.Configure(node.ID, nodewire.ConfigCommand{Core: "xray", Text: invalid}); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 20*time.Second, "invalid configuration rejected without losing VPN", func() bool {
		var n model.RemoteNode
		db.First(&n, node.ID)
		var report map[string]any
		json.Unmarshal([]byte(n.CoreReport), &report)
		return report["configGeneration"] == "2" && report["configError"] != "" && listener()
	})
	if err = svc.Configure(node.ID, nodewire.ConfigCommand{Core: "xray", Text: current}); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 20*time.Second, "valid configuration applied", func() bool {
		var n model.RemoteNode
		db.First(&n, node.ID)
		var report map[string]any
		json.Unmarshal([]byte(n.CoreReport), &report)
		return report["configGeneration"] == "3" && report["configError"] == "" && listener()
	})
	var after xray.ClientTraffic
	db.Where("email = ?", email).First(&after)
	if after.AllTime < before.AllTime || after.Total != before.Total {
		t.Fatal("runtime control changed quota/counters", before, after)
	}

	coreAck := func(generation string) bool {
		var n model.RemoteNode
		db.First(&n, node.ID)
		var report map[string]any
		json.Unmarshal([]byte(n.CoreReport), &report)
		if report["coreActionGeneration"] == generation && report["coreActionError"] != "" {
			t.Fatalf("core action failed: %v", report["coreActionError"])
		}
		return report["coreActionGeneration"] == generation
	}
	if err = svc.CoreAction(node.ID, nodewire.CoreAction{Core: "xray", Action: "stop"}); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 15*time.Second, "individual core stop acknowledged", func() bool { return coreAck("1") && !listener() })
	time.Sleep(11 * time.Second)
	if listener() {
		t.Fatal("stopped core restarted during routine sync")
	}
	if err = svc.CoreAction(node.ID, nodewire.CoreAction{Core: "xray", Action: "start"}); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 15*time.Second, "individual core start", func() bool { return coreAck("2") && listener() })
	if err = svc.CoreAction(node.ID, nodewire.CoreAction{Core: "xray", Action: "restart"}); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 15*time.Second, "individual core restart", func() bool { return coreAck("3") && listener() })
	radiusBound := func() bool {
		c, e := net.ListenPacket("udp", "127.0.0.1:1812")
		if e == nil {
			c.Close()
			return false
		}
		return true
	}
	if err = svc.CoreAction(node.ID, nodewire.CoreAction{Core: "radius", Action: "stop"}); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 15*time.Second, "RADIUS stops independently of native VPN", func() bool { return coreAck("4") && !radiusBound() && listener() })
	if err = svc.CoreAction(node.ID, nodewire.CoreAction{Core: "radius", Action: "start"}); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 15*time.Second, "RADIUS starts without restarting node", func() bool { return coreAck("5") && radiusBound() && listener() })

	// A real network outage exercises the complete 90-second admission lease.
	unreachable.Store(true)
	waitNode(t, 100*time.Second, "expired management lease stops VPN", func() bool { return !listener() })
	unreachable.Store(false)
	waitNode(t, 15*time.Second, "agent reconnects and resumes without reinstall", listener)
	if err = svc.CoreAction(node.ID, nodewire.CoreAction{Core: "xray", Action: "uninstall"}); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 15*time.Second, "individual core removed", func() bool {
		_, e := os.Stat(filepath.Join(folder, "bin", corebundle.XrayBinaryName()))
		return coreAck("6") && os.IsNotExist(e) && !listener()
	})
	var retained int64
	db.Model(&model.Inbound{}).Where("id = ?", in.Id).Count(&retained)
	if retained != 1 {
		t.Fatal("core removal deleted central inbound")
	}
	if err = svc.InstallCores(node.ID, []string{"xray"}); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 20*time.Second, "removed core can be reinstalled", listener)

	if err = svc.Update(node.ID, node.Name, node.Address, false); err != nil {
		t.Fatal(err)
	}
	waitNode(t, 15*time.Second, "disabling location stops its listener", func() bool {
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
		if e == nil {
			c.Close()
			return false
		}
		return true
	})
}
