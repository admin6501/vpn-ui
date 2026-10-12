package nodeagent

import (
	"encoding/json"
	"errors"
	"github.com/mhsanaei/3x-ui/v2/corebundle"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/web"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestNodeProvisionKeepsUnchangedRuntime(t *testing.T) {
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	snap := &nodewire.Snapshot{Version: nodewire.Version, NodeID: 1, LeaseSeconds: nodewire.LeaseSeconds, Enable: true}
	snap.Accounts = []nodewire.Account{{Account: model.Account{Id: 1, Email: "during-install", Enable: true}, Allowed: true, Remaining: 100, ReportedUsage: 5}}
	revision, err := service.ApplyNodeSnapshot(snap, map[string]bool{"xray": true})
	if err != nil {
		t.Fatal(err)
	}
	running := &web.Server{}
	a := &Agent{runtime: running, inventory: `{"xray":true}`, revision: revision, lastSuccess: time.Now().Add(-time.Minute)}
	previous := a.lastSuccess
	database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "during-install").Update("up", 5)
	snap.Accounts[0].Remaining = 0
	if err := a.syncWhileProvisioning(snap); err != nil {
		t.Fatal(err)
	}
	var traffic xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", "during-install").First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Enable || traffic.Total != 5 {
		t.Fatal("quota exhaustion not applied during installation", traffic)
	}
	if a.runtime != running {
		t.Fatal("additional core installation stopped the existing runtime")
	}
	if !a.lastSuccess.After(previous) {
		t.Fatal("successful provisioning sync did not renew the lease")
	}
}

func TestNodeProvisionLargeBuildLogFitsReport(t *testing.T) {
	source := []service.ProvisionStep{}
	for i := 0; i < 30; i++ {
		source = append(source, service.ProvisionStep{Name: "build", OK: false, Log: strings.Repeat("x", 100000) + "BUILD FAILED"})
	}
	trimmed := boundedProvisionLogs(source)
	raw, err := json.Marshal(trimmed)
	if err != nil || len(raw) > 256<<10 {
		t.Fatal("build log still exceeds report budget", len(raw), err)
	}
	if !strings.HasSuffix(trimmed[len(trimmed)-1].Log, "BUILD FAILED") {
		t.Fatal("error tail lost")
	}
	if len(source[0].Log) != 100012 {
		t.Fatal("trimming changed the local full log")
	}
}

func TestNodeProvisionExplicitXrayReinstallSchedulesActivation(t *testing.T) {
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	running := &web.Server{}
	a := &Agent{runtime: running}
	request := &nodewire.Snapshot{CoreGeneration: 1, RequestedCores: []string{"xray"}}
	if err := a.requestCores(request); err != nil {
		t.Fatal(err)
	}
	if a.runtime != running || !a.provisionRefresh || a.coreRestart != "xray" || !a.coreRestartForce {
		t.Fatal("explicit core reinstall did not preserve runtime and schedule new binary activation")
	}
	a.provisionRefresh = false
	a.coreRestart = ""
	if err := a.requestCores(request); err != nil {
		t.Fatal(err)
	}
	if a.provisionRefresh || a.coreRestart != "" {
		t.Fatal("acknowledged install repeated on next sync")
	}
}

func TestNodeUninstallSSHLeavesRuntimeAndSiblingAdmission(t *testing.T) {
	t.Setenv("VPNUI_NODE_AGENT", "1")
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	if err := service.SetNodeSetting(database.GetDB(), "nodeSSHInstalled", "true"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.GetBinFolderPath(), corebundle.XrayBinaryName()), []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	in := model.Inbound{Id: 1, Protocol: model.VLESS, Enable: true, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(&in).Error; err != nil {
		t.Fatal(err)
	}
	running := &web.Server{}
	a := &Agent{runtime: running}
	if err := a.executeCoreAction(nodewire.CoreAction{Core: "ssh", Action: "uninstall"}); err != nil {
		t.Fatal(err)
	}
	if a.runtime != running {
		t.Fatal("removing SSH stopped unrelated node runtime")
	}
	var retained model.Inbound
	if err := database.GetDB().First(&retained, in.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !retained.Enable {
		t.Fatal("removing SSH disabled unrelated VLESS admission")
	}
	if !service.NodeCorePaused("ssh") || service.GetNodeSetting("nodeSSHInstalled") != "false" {
		t.Fatal("SSH removal was not recorded")
	}
}

func TestNodeSyncErrorReportsDoNotExposeCredentials(t *testing.T) {
	for _, operation := range []string{"exchange", "snapshot", "runtime", "installation", "unknown"} {
		secret := "private-key-and-pair-token-fixture"
		err := atSyncStage(operation, errors.New(secret))
		if strings.Contains(publicSyncError(err), secret) {
			t.Fatal("diagnostic leaked private configuration")
		}
		if !strings.Contains(err.Error(), secret) {
			t.Fatal("local diagnostic detail lost")
		}
	}
	if !strings.Contains(publicSyncError(atSyncStage("runtime", errors.New("fixture"))), "core configuration/start") {
		t.Fatal("runtime failure still reported as network failure")
	}
}
