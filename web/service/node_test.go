package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func nodeFixture(t *testing.T) (*NodeService, *model.RemoteNode, string) {
	t.Helper()
	newAccountsDB(t)
	svc := &NodeService{}
	n, pair, e := svc.Create("Germany", "de.example.com")
	if e != nil {
		t.Fatal(e)
	}
	return svc, n, pair
}
func TestNodeEnrollmentIsSingleUseAndScoped(t *testing.T) {
	svc, n, pair := nodeFixture(t)
	token := strings.Repeat("a", 64)
	request := nodewire.Enrollment{PairToken: pair, AgentToken: token, Version: "test", Architecture: "arm64"}
	id, e := svc.Enroll(request)
	if e != nil || id != n.ID {
		t.Fatalf("enroll: %v", e)
	}
	if _, e = svc.Enroll(request); e != nil {
		t.Fatal("lost response retry rejected", e)
	}
	request.AgentToken = strings.Repeat("b", 64)
	if _, e = svc.Enroll(request); e == nil {
		t.Fatal("pairing token reused")
	}
	if _, e = svc.Authenticate(n.ID, token); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Authenticate(n.ID+1, token); e == nil {
		t.Fatal("node token accessed another node")
	}
	var stored model.RemoteNode
	database.GetDB().First(&stored, n.ID)
	raw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), pair) || strings.Contains(string(raw), token) || stored.TokenHash == token {
		t.Fatal("credential leaked")
	}
}
func seedNodeAccount(t *testing.T, node *model.RemoteNode, port int, email string) (*model.Inbound, *model.Account) {
	t.Helper()
	in := seedInboundWithClients(t, model.VLESS, port, []map[string]any{{"id": "11111111-2222-3333-4444-555555555555", "email": email, "subId": "node-test-sub", "enable": true, "totalGB": float64(10000)}})
	if e := database.GetDB().Model(in).Updates(map[string]any{"node_id": node.ID, "tag": nodeInboundTag(node.ID, in.Tag)}).Error; e != nil {
		t.Fatal(e)
	}
	(&AccountService{}).MigrationAccounts()
	a, e := (&AccountService{}).GetAccountByEmail(email)
	if e != nil {
		t.Fatal(e)
	}
	seedTraffic(t, in.Id, email, 0, 0, a.TotalGB, a.ExpiryTime, true)
	return in, a
}
func TestNodeCumulativeBillingIsExactlyOnceAcrossLocations(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	in, a := seedNodeAccount(t, n, 48501, "one@example.com")
	other, _, e := svc.Create("Finland", "fi.example.com")
	if e != nil {
		t.Fatal(e)
	}
	second := seedInboundWithClients(t, model.Trojan, 48502, []map[string]any{})
	database.GetDB().Model(second).Updates(map[string]any{"node_id": other.ID, "tag": nodeInboundTag(other.ID, second.Tag)})
	a.Password = "test-password"
	database.GetDB().Save(a)
	if _, e = (&AccountService{}).ApplyMemberships(a.Email, []int{in.Id, second.Id}, nil, true); e != nil {
		t.Fatal(e)
	}
	report := nodewire.Report{Version: 1, Sequence: 1, Counters: []nodewire.Counter{{Kind: "account", AccountID: a.Id, Up: 100, Down: 200}, {Kind: "membership", AccountID: a.Id, InboundID: in.Id, Up: 100, Down: 200}}, Online: []nodewire.Online{{AccountID: a.Id, InboundID: in.Id}}}
	if e = svc.AcceptReport(n.ID, report); e != nil {
		t.Fatal(e)
	}
	if e = svc.AcceptReport(n.ID, report); e != nil {
		t.Fatal(e)
	}
	report.Counters = []nodewire.Counter{{Kind: "account", AccountID: a.Id, Up: 40, Down: 60}}
	report.Online = []nodewire.Online{{AccountID: a.Id, InboundID: second.Id}}
	if e = svc.AcceptReport(other.ID, report); e != nil {
		t.Fatal(e)
	}
	var trow xray.ClientTraffic
	database.GetDB().Where("email = ?", a.Email).First(&trow)
	if trow.Up != 140 || trow.Down != 260 || trow.AllTime != 400 {
		t.Fatalf("duplicated/missing usage: %+v", trow)
	}
	online := (&InboundService{}).GetOnlineClients()
	if len(online) != 1 {
		t.Fatalf("unique online count = %d", len(online))
	}
	report.Sequence = 2
	report.Counters[0].Up = 50
	report.Counters[0].Down = 70
	if e = svc.AcceptReport(other.ID, report); e != nil {
		t.Fatal(e)
	}
	database.GetDB().Where("email = ?", a.Email).First(&trow)
	if trow.Up+trow.Down != 420 {
		t.Fatal("cumulative delta wrong")
	}
	report.Sequence = 3
	report.Counters[0].Up = 1
	if e = svc.AcceptReport(other.ID, report); e == nil {
		t.Fatal("backwards counter accepted")
	}
	database.GetDB().Where("email = ?", a.Email).First(&trow)
	if trow.Up+trow.Down != 420 {
		t.Fatal("failed report partially committed")
	}
}
func TestNodeReportsRejectForeignAccountsAndSequenceGaps(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	in, a := seedNodeAccount(t, n, 48511, "own@example.com")
	other, _, _ := svc.Create("Other", "other.example.com")
	_, foreign := seedNodeAccount(t, other, 48512, "foreign@example.com")
	report := nodewire.Report{Version: 1, Sequence: 1, Counters: []nodewire.Counter{{Kind: "account", AccountID: a.Id, Up: 10}, {Kind: "account", AccountID: foreign.Id, Down: 90}}}
	if e := svc.AcceptReport(n.ID, report); e == nil {
		t.Fatal("foreign account accepted")
	}
	var traffic xray.ClientTraffic
	database.GetDB().Where("email = ?", a.Email).First(&traffic)
	if traffic.Up != 0 {
		t.Fatal("partial transaction")
	}
	report.Sequence = 2
	report.Counters = report.Counters[:1]
	if e := svc.AcceptReport(n.ID, report); e == nil {
		t.Fatal("sequence gap accepted")
	}
	if e := svc.Delete(n.ID); e == nil {
		t.Fatal("node with inbounds deleted")
	}
	if exists, e := (&InboundService{}).checkPortExistOnNode(other.ID, "", in.Port, 0); e != nil || exists {
		t.Fatal(e)
	}
}
func TestNodeSnapshotScopeAndNoAutomaticCores(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	_, a := seedNodeAccount(t, n, 48521, "visible@example.com")
	other, _, _ := svc.Create("Other", "other.example.com")
	_, foreign := seedNodeAccount(t, other, 48522, "hidden@example.com")
	a.ProtocolCredentials = `{"vless":{"id":"22222222-3333-4444-5555-666666666666"}}`
	database.GetDB().Save(a)
	snap, e := svc.Snapshot(n.ID)
	if e != nil {
		t.Fatal(e)
	}
	if len(snap.Accounts) != 1 || snap.Accounts[0].Account.Id != a.Id || len(snap.Inbounds) != 1 || len(snap.RequestedCores) != 0 {
		t.Fatal("unscoped snapshot or implicit core installation")
	}
	raw, _ := json.Marshal(snap)
	if strings.Contains(string(raw), foreign.Email) || snap.Accounts[0].ProtocolCredentials == "" {
		t.Fatal("scope/credential projection failed")
	}
	if e = svc.InstallCores(n.ID, nil); e == nil {
		t.Fatal("empty selection installs all")
	}
	if e = svc.InstallCores(n.ID, []string{"openvpn"}); e != nil {
		t.Fatal(e)
	}
	snap, e = svc.Snapshot(n.ID)
	if e != nil || len(snap.RequestedCores) != 1 || snap.RequestedCores[0] != "openvpn" {
		t.Fatal("selected core expanded to all cores")
	}
	database.GetDB().Model(n).Update("last_seen", time.Now().Add(-time.Minute).UnixMilli())
	if clients, _ := nodeOnline(database.GetDB()); len(clients) != 0 {
		t.Fatal("stale online snapshot")
	}
}
func TestNodeWorkerPreservesMeterAndImportedCredentials(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	in, a := seedNodeAccount(t, n, 48531, "imported@example.com")
	a.ProtocolCredentials = `{"vless":{"id":"22222222-3333-4444-5555-666666666666"}}`
	database.GetDB().Save(a)
	snap, e := svc.Snapshot(n.ID)
	if e != nil {
		t.Fatal(e)
	}
	// This test switches the same fixture into worker mode without running host
	// daemons. Real network integration uses independent panel/agent processes.
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if _, e = ApplyNodeSnapshot(snap, map[string]bool{}); e != nil {
		t.Fatal(e)
	}
	var worker model.Inbound
	database.GetDB().First(&worker, in.Id)
	if worker.Enable {
		t.Fatal("uninstalled core started")
	}
	if _, e = ApplyNodeSnapshot(snap, map[string]bool{"xray": true}); e != nil {
		t.Fatal(e)
	}
	var traffic xray.ClientTraffic
	database.GetDB().Where("email = ?", a.Email).First(&traffic)
	traffic.Up = 110
	traffic.Down = 10
	traffic.AllTime = 120
	database.GetDB().Save(&traffic)
	snap.Accounts[0].Remaining = 900
	snap.Accounts[0].ReportedUsage = 100
	if _, e = ApplyNodeSnapshot(snap, map[string]bool{"xray": true}); e != nil {
		t.Fatal(e)
	}
	database.GetDB().Where("email = ?", a.Email).First(&traffic)
	if traffic.Up != 110 || traffic.Down != 10 || traffic.Total != 1000 {
		t.Fatal("meter reset or wrong remaining budget")
	}
	database.GetDB().First(&worker, in.Id)
	var settings map[string]any
	json.Unmarshal([]byte(worker.Settings), &settings)
	clients := settings["clients"].([]any)
	if clients[0].(map[string]any)["id"] != "22222222-3333-4444-5555-666666666666" {
		t.Fatal("imported credential overwritten")
	}
	if e = DisableNodeAdmission(); e != nil {
		t.Fatal(e)
	}
	database.GetDB().First(&worker, in.Id)
	if worker.Enable {
		t.Fatal("expired lease kept serving")
	}
	database.GetDB().Where("email = ?", a.Email).First(&traffic)
	if traffic.Up+traffic.Down != 120 {
		t.Fatal("lease expiry erased meter")
	}
}

func TestNodeCoreMappingCoversEveryPanelProtocol(t *testing.T) {
	for _, tc := range []struct {
		protocol model.Protocol
		core     string
	}{
		{model.VLESS, "xray"}, {model.VMESS, "xray"}, {model.Trojan, "xray"}, {model.Shadowsocks, "xray"},
		{model.HTTP, "xray"}, {model.Mixed, "xray"}, {model.Tunnel, "xray"}, {model.Hysteria2, "xray"}, {model.WireGuard, "xray"}, {model.Hysteria, "xray"}, {model.TUIC, "xray"}, {model.ANYTLS, "xray"}, {model.NAIVE, "xray"},
		{model.OPENVPN, "openvpn"}, {model.L2TP, "l2tp"}, {model.PPTP, "pptp"}, {model.OPENCONNECT, "openconnect"},
		{model.SSTP, "sstp"}, {model.IKEV2, "ikev2"}, {model.WGC, "wgc"}, {model.AWG, "awg"}, {model.GRE, "gre"}, {model.MTPROTO, "mtproto"}, {model.SSH, "ssh"},
	} {
		if got := NodeCoreForProtocol(tc.protocol); got != tc.core {
			t.Errorf("%s mapped to %s, want %s", tc.protocol, got, tc.core)
		}
	}
}

func TestNodeCertificateBundleCoversOpenVpnAndRejectsInvalidPaths(t *testing.T) {
	directory := t.TempDir()
	cert := filepath.Join(directory, "certificate.pem")
	if err := os.WriteFile(cert, []byte("synthetic-test-certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	settings := map[string]any{"tlsUseFile": true, "caCertFile": cert, "serverCertFile": cert, "serverKeyFile": cert, "tlsCryptFile": cert, "unrelatedPath": "/must/not/be/read"}
	raw, _ := json.Marshal(settings)
	in := model.Inbound{Settings: string(raw)}
	files := map[string][]byte{}
	if err := snapshotCertificateFiles(&in, files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatal("certificate content not deduplicated")
	}
	var projected map[string]any
	json.Unmarshal([]byte(in.Settings), &projected)
	for _, key := range []string{"caCertFile", "serverCertFile", "serverKeyFile", "tlsCryptFile"} {
		if !strings.HasPrefix(projected[key].(string), "nodefile:") {
			t.Errorf("%s not transferred", key)
		}
	}
	if projected["unrelatedPath"] != "/must/not/be/read" {
		t.Fatal("arbitrary file path treated as a certificate")
	}
	in.Settings = `{"serverKeyFile":"/does-not-exist"}`
	if snapshotCertificateFiles(&in, files) == nil {
		t.Fatal("missing required certificate ignored")
	}
}

func TestNodeWorkerAccountRenameKeepsCumulativeCounter(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	_, account := seedNodeAccount(t, n, 48611, "before@example.com")
	snap, err := svc.Snapshot(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if _, err = ApplyNodeSnapshot(snap, map[string]bool{"xray": true}); err != nil {
		t.Fatal(err)
	}
	if err = database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", account.Email).Update("up", 100).Error; err != nil {
		t.Fatal(err)
	}
	snap.Accounts[0].Account.Email = "after@example.com"
	if _, err = ApplyNodeSnapshot(snap, map[string]bool{"xray": true}); err != nil {
		t.Fatal(err)
	}
	counters, err := ReadNodeCounters(database.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	for _, counter := range counters {
		if counter.Kind == "account" && counter.AccountID == account.Id {
			if counter.Up != 100 {
				t.Fatal("rename reset cumulative usage")
			}
			return
		}
	}
	t.Fatal("renamed account lost its counter")
}

func TestNodeControlAndConfigurationAreScoped(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	other, _, _ := svc.Create("Other", "other.example.com")
	if err := svc.Control(n.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	snap, err := svc.Snapshot(n.ID)
	if err != nil || snap.Enable || snap.ControlGeneration != 1 {
		t.Fatal("stop not persistent", snap, err)
	}
	if err = svc.Configure(n.ID, nodewire.ConfigCommand{Core: "xray", Text: `{}`}); err == nil {
		t.Fatal("configuration allowed on stopped runtime")
	}
	if err = svc.Configure(n.ID, nodewire.ConfigCommand{Core: "xray", Inspect: true}); err != nil {
		t.Fatal(err)
	}
	if err = svc.Control(n.ID, "start"); err != nil {
		t.Fatal(err)
	}
	if err = svc.Control(n.ID, "restart"); err != nil {
		t.Fatal(err)
	}
	if err = svc.Control(n.ID, "unknown"); err == nil {
		t.Fatal("unknown control accepted")
	}
	if err = svc.Configure(n.ID, nodewire.ConfigCommand{Core: "xray", Text: "invalid"}); err == nil {
		t.Fatal("invalid template accepted")
	}
	snap, err = svc.Snapshot(n.ID)
	if err != nil || !snap.Enable || snap.ControlGeneration != 3 || snap.ConfigGeneration != 1 || snap.ConfigCommand == nil || !snap.ConfigCommand.Inspect {
		t.Fatal("commands changed unexpectedly", snap, err)
	}
	untouched, err := svc.Snapshot(other.ID)
	if err != nil || !untouched.Enable || untouched.ControlGeneration != 0 || untouched.ConfigGeneration != 0 {
		t.Fatal("other node changed", untouched, err)
	}
}

func TestNodeInboundConfigurationRetainsOwnershipCredentialsAndQuota(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	in, a := seedNodeAccount(t, n, 48731, "configuration@example.com")
	originalOwner := in.UserId
	targets := NodeInboundConfigurationTargets(n.ID, "xray")
	if len(targets) != 1 {
		t.Fatal(targets)
	}
	var editable nodeInboundConfiguration
	if err := json.Unmarshal([]byte(targets[0]["current"].(string)), &editable); err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	json.Unmarshal(editable.Settings, &settings)
	if _, ok := settings["clients"]; ok {
		t.Fatal("account credentials exposed in editable settings")
	}
	settings["decryption"] = "none"
	editable.Settings, _ = json.Marshal(settings)
	raw, _ := json.Marshal(editable)
	if err := svc.Configure(n.ID, nodewire.ConfigCommand{Core: "xray", Inbound: in.Id, File: "inbound.json", Text: string(raw)}); err != nil {
		t.Fatal(err)
	}
	var stored model.Inbound
	database.GetDB().First(&stored, in.Id)
	var updated model.Account
	database.GetDB().First(&updated, a.Id)
	if stored.NodeID != n.ID || stored.UserId != originalOwner || updated.TotalGB != a.TotalGB || updated.SubID != a.SubID {
		t.Fatal("configuration changed ownership/quota/token", updated, a)
	}
	var projected map[string]any
	json.Unmarshal([]byte(stored.Settings), &projected)
	if len(projected["clients"].([]any)) != 1 {
		t.Fatal("clients lost")
	}
	other, _, _ := svc.Create("Other", "other.example.com")
	if err := svc.Configure(other.ID, nodewire.ConfigCommand{Core: "xray", Inbound: in.Id, File: "inbound.json", Text: string(raw)}); err == nil {
		t.Fatal("foreign inbound edited")
	}
}
func TestNodeTemplateProtectsAccounting(t *testing.T) {
	if err := ValidateNodeTemplate(defaultNodeTemplate); err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	json.Unmarshal([]byte(defaultNodeTemplate), &cfg)
	delete(cfg, "api")
	raw, _ := json.Marshal(cfg)
	if err := ValidateNodeTemplate(string(raw)); err == nil {
		t.Fatal("accounting API removal accepted")
	}
}

func TestNodeCorePauseSurvivesSnapshotAndRetainsMeter(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	in, a := seedNodeAccount(t, n, 48741, "pause@example.com")
	snap, err := svc.Snapshot(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VPNUI_NODE_AGENT", "1")
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if err = SetNodeCorePaused("xray", true); err != nil {
		t.Fatal(err)
	}
	if _, err = ApplyNodeSnapshot(snap, map[string]bool{"xray": true}); err != nil {
		t.Fatal(err)
	}
	var stored model.Inbound
	database.GetDB().First(&stored, in.Id)
	if stored.Enable {
		t.Fatal("snapshot resumed stopped core")
	}
	var traffic xray.ClientTraffic
	database.GetDB().Where("email = ?", a.Email).First(&traffic)
	traffic.Down = 50
	database.GetDB().Save(&traffic)
	if err = SetNodeCorePaused("xray", false); err != nil {
		t.Fatal(err)
	}
	if _, err = ApplyNodeSnapshot(snap, map[string]bool{"xray": true}); err != nil {
		t.Fatal(err)
	}
	database.GetDB().First(&stored, in.Id)
	database.GetDB().Where("email = ?", a.Email).First(&traffic)
	if !stored.Enable || traffic.Down != 50 {
		t.Fatal("resume did not retain data", stored.Enable, traffic.Down)
	}
}

func TestNodeInspectionDoesNotRestartDataPlane(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	seedNodeAccount(t, n, 48751, "inspection@example.com")
	snap, err := svc.Snapshot(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	before, err := ApplyNodeSnapshot(snap, map[string]bool{"xray": true})
	if err != nil {
		t.Fatal(err)
	}
	snap.ConfigGeneration = 42
	snap.ConfigCommand = &nodewire.ConfigCommand{Core: "xray", Inspect: true}
	snap.CoreActionGeneration = 43
	snap.CoreActionCommand = &nodewire.CoreAction{Core: "xray", Action: "start"}
	after, err := ApplyNodeSnapshot(snap, map[string]bool{"xray": true})
	if err != nil || before != after {
		t.Fatal("management command changed data-plane revision", before, after, err)
	}
}

func TestNodeControlsUpgradeExistingNodeRows(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	db := database.GetDB()
	for _, column := range []string{"paused", "control_generation", "config_generation", "core_action_generation"} {
		if err := db.Migrator().DropColumn(&model.RemoteNode{}, column); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AutoMigrate(&model.RemoteNode{}); err != nil {
		t.Fatal(err)
	}
	var existing int64
	db.Model(&model.RemoteNode{}).Where("id = ? AND paused = ?", n.ID, false).Count(&existing)
	if existing != 1 {
		t.Fatal("upgraded node has no usable paused default")
	}
	if err := svc.Control(n.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	snap, err := svc.Snapshot(n.ID)
	if err != nil || snap.ControlGeneration != 1 || snap.Enable {
		t.Fatal("upgraded row cannot receive commands", snap, err)
	}
}

func TestNodeWorkerPublishesClientSpeedOverridesBothDirections(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	_, account := seedNodeAccount(t, n, 48901, "limited@example.com")
	down, up := 128, 64
	account.SpeedLimitDown, account.SpeedLimitUp = &down, &up
	if err := database.GetDB().Save(account).Error; err != nil {
		t.Fatal(err)
	}
	snap, err := svc.Snapshot(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if _, err := ApplyNodeSnapshot(snap, map[string]bool{"xray": true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(xray.GetSpeedLimitPath())
	if err != nil {
		t.Fatal(err)
	}
	var doc speedLimitDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Users) != 1 || doc.Users[0].Email != account.Email || doc.Users[0].DownBps != 128*1024 || doc.Users[0].UpBps != 64*1024 {
		t.Fatalf("limits not delivered to worker: %s", data)
	}
}
