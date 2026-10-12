package nodeagent

import (
	"encoding/json"
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
