// Package nodeagent runs the outbound-only agent and the panel's shared data plane.
package nodeagent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v2/backend"
	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/corebundle"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/web"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/op/go-logging"
	"gorm.io/gorm"
)

type Config struct {
	PanelURL string `json:"panelUrl"`
	Token    string `json:"token"`
	NodeID   int    `json:"nodeId"`
	CAFile   string `json:"caFile,omitempty"`
}

func ValidatePanelURL(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("Invalid panel URL")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := ip != nil && ip.IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return "", errors.New("Use HTTPS for node management / ارتباط مدیریت نود باید HTTPS باشد")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/"
	return u.String(), nil
}
func Client(c Config) (*http.Client, error) {
	roots, e := x509.SystemCertPool()
	if e != nil {
		roots = x509.NewCertPool()
	}
	if c.CAFile != "" {
		pem, e := os.ReadFile(c.CAFile)
		if e != nil {
			return nil, e
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("Invalid CA file")
		}
	}
	return &http.Client{Timeout: 25 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("Panel URL redirected; use its exact HTTPS address")
	}}, nil
}
func saveConfig(path string, c Config) error {
	data, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	if e = os.WriteFile(path+".tmp", data, 0600); e != nil {
		return e
	}
	return os.Rename(path+".tmp", path)
}
func exchange(ctx context.Context, client *http.Client, c Config, path string, input, output any) error {
	body, e := json.Marshal(input)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.PanelURL+"node-agent/"+path, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	if c.NodeID > 0 {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	response, e := client.Do(req)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Panel returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(output)
}
func Enroll(ctx context.Context, path, panel, pair, ca string) error {
	panel, e := ValidatePanelURL(panel)
	if e != nil {
		return e
	}
	if len(pair) != 64 {
		return errors.New("Invalid pairing token")
	}
	// Persist the generated token BEFORE enrollment: a lost HTTP response retries
	// the same credential instead of consuming the single-use code twice.
	c := Config{PanelURL: panel, CAFile: ca}
	if data, e := os.ReadFile(path); e == nil {
		if json.Unmarshal(data, &c) != nil {
			return errors.New("Invalid existing agent config")
		}
		if c.NodeID > 0 {
			if c.PanelURL != panel {
				return errors.New("Agent is paired to another panel")
			}
			fmt.Println("Agent already paired / عامل قبلاً متصل شده است")
			return nil
		}
		if c.PanelURL != panel {
			return errors.New("Pending enrollment belongs to another panel")
		}
	}
	if c.Token == "" {
		secret := make([]byte, 32)
		if _, e = rand.Read(secret); e != nil {
			return e
		}
		c.Token = hex.EncodeToString(secret)
	}
	if e = saveConfig(path, c); e != nil {
		return e
	}
	client, e := Client(c)
	if e != nil {
		return e
	}
	var reply nodewire.EnrollmentReply
	if e = exchange(ctx, client, c, "enroll", nodewire.Enrollment{PairToken: pair, AgentToken: c.Token, Version: config.GetVersion(), Architecture: runtime.GOARCH}, &reply); e != nil {
		return e
	}
	if reply.NodeID <= 0 {
		return errors.New("Invalid node id")
	}
	c.NodeID = reply.NodeID
	if e = saveConfig(path, c); e != nil {
		return e
	}
	fmt.Printf("Node connected / نود متصل شد: #%d\n", c.NodeID)
	return nil
}

type Agent struct {
	config      Config
	client      *http.Client
	runtime     *web.Server
	lastSuccess time.Time
	revision    string
	inventory   string
	lastError   string
}

func New(path string) (*Agent, error) {
	data, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var c Config
	if e = json.Unmarshal(data, &c); e != nil {
		return nil, e
	}
	panel, e := ValidatePanelURL(c.PanelURL)
	if e != nil {
		return nil, e
	}
	c.PanelURL = panel
	if c.NodeID <= 0 || len(c.Token) != 64 {
		return nil, errors.New("Enroll the node first")
	}
	client, e := Client(c)
	if e != nil {
		return nil, e
	}
	return &Agent{config: c, client: client}, nil
}
func prepareDatabase() error {
	os.Setenv("VPNUI_NODE_AGENT", "1")
	logger.InitLogger(logging.INFO)
	if e := database.InitDB(config.GetDBPath()); e != nil {
		return e
	}
	if e := os.Chmod(config.GetDBPath(), 0600); e != nil {
		return e
	}
	if backend.Available() {
		if _, e := service.RefreshInstalledDaemons(); e != nil {
			return e
		}
	}
	// An agent may resume only after a fresh authenticated lease. Never start VPNs
	// with cached permissions merely because systemd restarted the process.
	return service.DisableNodeAdmission()
}
func installedCores() map[string]bool {
	set := map[string]bool{}
	if _, e := os.Stat(filepath.Join(config.GetBinFolderPath(), corebundle.XrayBinaryName())); e == nil {
		set["xray"] = true
	}
	for _, c := range (&service.CoreService{}).CoreCatalog() {
		if c.Installed && !c.Builtin {
			set[c.Name] = true
		}
	}
	if service.GetNodeSetting("nodeSSHInstalled") == "true" {
		set["ssh"] = true
	}
	return set
}
func coreReport() json.RawMessage {
	cs := &service.CoreService{}
	installed := []string{}
	for name := range installedCores() {
		installed = append(installed, name)
	}
	sort.Strings(installed)
	report := map[string]any{"installed": installed, "catalog": cs.CoreCatalog(), "cores": cs.GetCoresStatus(), "system": cs.GetSystemStatus(), "provision": cs.ProvisionState(), "generation": service.GetNodeSetting("nodeCoreGeneration")}
	data, _ := json.Marshal(report)
	return data
}
func (a *Agent) pendingReport() (nodewire.Report, error) {
	var result nodewire.Report
	e := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var row model.Setting
		if tx.Where("key = ?", "nodePendingReport").First(&row).Error == nil && row.Value != "" {
			return json.Unmarshal([]byte(row.Value), &result)
		}
		var seq model.Setting
		tx.Where("key = ?", "nodeReportSequence").First(&seq)
		number, _ := strconv.ParseInt(seq.Value, 10, 64)
		counters, e := service.ReadNodeCounters(tx)
		if e != nil {
			return e
		}
		result = nodewire.Report{Version: nodewire.Version, Sequence: number + 1, Counters: counters, Online: service.ReadNodeOnline(), CoreReport: coreReport(), LastError: a.lastError}
		raw, e := json.Marshal(result)
		if e != nil {
			return e
		}
		return service.SetNodeSetting(tx, "nodePendingReport", string(raw))
	})
	return result, e
}
func ackReport(sequence int64) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if e := service.SetNodeSetting(tx, "nodeReportSequence", strconv.FormatInt(sequence, 10)); e != nil {
			return e
		}
		return service.SetNodeSetting(tx, "nodePendingReport", "")
	})
}
func (a *Agent) requestCores(snap *nodewire.Snapshot) error {
	generation := strconv.FormatInt(snap.CoreGeneration, 10)
	if snap.CoreGeneration == 0 || service.GetNodeSetting("nodeCoreGeneration") == generation {
		return nil
	}
	if len(snap.RequestedCores) == 0 {
		return errors.New("Empty core request rejected")
	}
	cs := &service.CoreService{}
	if cs.ProvisionState().Running {
		return nil
	}
	a.stopRuntime()
	// Xray is a dependency of VPN accounting/routing, not a selection that installs
	// every other core. Nothing is extracted before the first central request.
	if _, e := corebundle.ExtractXray(config.GetBinFolderPath()); e != nil {
		return e
	}
	if _, e := corebundle.ExtractGeofiles(config.GetBinFolderPath()); e != nil {
		return e
	}
	installable := []string{}
	for _, c := range snap.RequestedCores {
		switch c {
		case "xray":
		case "ssh":
			if e := service.SetNodeSetting(database.GetDB(), "nodeSSHInstalled", "true"); e != nil {
				return e
			}
		default:
			installable = append(installable, c)
		}
	}
	if len(installable) > 0 && !cs.StartProvision(installable) {
		return errors.New("Core installation busy")
	}
	return service.SetNodeSetting(database.GetDB(), "nodeCoreGeneration", generation)
}
func (a *Agent) Tick(ctx context.Context) error {
	timeout := 25 * time.Second
	if a.runtime != nil && !a.lastSuccess.IsZero() {
		remaining := time.Until(a.lastSuccess.Add(nodewire.LeaseSeconds * time.Second))
		if remaining <= 0 {
			a.stopRuntime()
		} else if remaining < timeout {
			timeout = remaining
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	report, e := a.pendingReport()
	if e != nil {
		return e
	}
	var snap nodewire.Snapshot
	if e = exchange(ctx, a.client, a.config, strconv.Itoa(a.config.NodeID)+"/sync", report, &snap); e != nil {
		return e
	}
	if snap.NodeID != a.config.NodeID || snap.Version != nodewire.Version {
		return errors.New("Wrong node snapshot")
	}
	if e = ackReport(report.Sequence); e != nil {
		return e
	}
	if e = a.requestCores(&snap); e != nil {
		return e
	}
	if (&service.CoreService{}).ProvisionState().Running {
		a.lastSuccess = time.Now()
		a.stopRuntime()
		return nil
	}
	installed := installedCores()
	inventoryRaw, _ := json.Marshal(installed)
	inventory := string(inventoryRaw)
	revision, e := service.ApplyNodeSnapshot(&snap, installed)
	if e != nil {
		return e
	}
	a.lastSuccess = time.Now()
	if !snap.Enable {
		a.stopRuntime()
		return nil
	}
	if len(installed) == 0 {
		return nil
	}
	if a.runtime == nil {
		a.runtime = web.NewServer()
		if e = a.runtime.StartNodeRuntime(); e != nil {
			a.stopRuntime()
			return e
		}
	} else if revision != a.revision || inventory != a.inventory {
		if e = a.runtime.RefreshNodeRuntime(); e != nil {
			return e
		}
	}
	a.revision = revision
	a.inventory = inventory
	a.lastError = ""
	return nil
}
func (a *Agent) stopRuntime() {
	if e := service.DisableNodeAdmission(); e != nil {
		log.Printf("disable node admission: %v", e)
	}
	if a.runtime != nil {
		if e := a.runtime.StopNodeRuntime(); e != nil {
			log.Printf("stop node runtime: %v", e)
		}
		a.runtime = nil
	}
}
func (a *Agent) Run(ctx context.Context) error {
	if e := prepareDatabase(); e != nil {
		return e
	}
	defer a.stopRuntime()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if e := a.Tick(ctx); e != nil {
			// Never print configuration bodies or bearer/pairing credentials.
			a.lastError = "Node synchronization failed; inspect agent journal"
			log.Printf("node synchronization: %v", e)
			if a.lastSuccess.IsZero() || time.Since(a.lastSuccess) >= nodewire.LeaseSeconds*time.Second {
				a.stopRuntime()
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
