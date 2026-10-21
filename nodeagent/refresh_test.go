package nodeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/web"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

func TestNodeSnapshotRefreshOnlyAffectedCores(t *testing.T) {
	snap := &nodewire.Snapshot{Enable: true,
		Inbounds:    []model.Inbound{{Id: 1, Protocol: model.L2TP, Enable: true}, {Id: 2, Protocol: model.GRE, Enable: true}},
		Accounts:    []nodewire.Account{{Account: model.Account{Id: 10, Email: "l2tp-user"}, Allowed: true, Remaining: 100}},
		Memberships: []model.AccountInbound{{AccountId: 10, InboundId: 1}},
	}
	before := snapshotCoreRevisions(snap)
	snap.Accounts[0].Remaining = 90
	snap.Accounts[0].ReportedUsage = 10
	snap.Inbounds[0].Up = 10
	snap.Memberships[0].Down = 10
	snap.ConfigGeneration = 1
	if got := changedSnapshotCores(before, snapshotCoreRevisions(snap)); len(got) != 0 {
		t.Fatal("meter/management-only update restarted services", got)
	}
	snap.Accounts[0].Allowed = false
	if got := changedSnapshotCores(before, snapshotCoreRevisions(snap)); !reflect.DeepEqual(got, []string{"l2tp", "xray"}) {
		t.Fatal("client disable must update L2TP and routing without restarting GRE", got)
	}
	before = snapshotCoreRevisions(snap)
	snap.Inbounds = snap.Inbounds[:1]
	if got := changedSnapshotCores(before, snapshotCoreRevisions(snap)); !reflect.DeepEqual(got, []string{"gre", "xray"}) {
		t.Fatal("removed inbound did not reconcile its old core", got)
	}
}

func TestNodeUnchangedHeartbeatSkipsClientRewritesButAppliesQuotaAndSpeedUsage(t *testing.T) {
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	usage := int64(0)
	snap := &nodewire.Snapshot{Version: nodewire.Version, NodeID: 1, LeaseSeconds: nodewire.LeaseSeconds, Enable: true,
		Accounts: []nodewire.Account{{Account: model.Account{Id: 1, Email: "cached-user", Enable: true}, Allowed: true, Remaining: 100, Usage: &usage}},
	}
	a := &Agent{runtime: &web.Server{}}
	installed := map[string]bool{"xray": true}
	if _, err := a.applySnapshot(snap, installed); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", "cached-user").Update("up", 5).Error; err != nil {
		t.Fatal(err)
	}
	writes := 0
	if err := db.Callback().Update().After("gorm:update").Register("test_node_heartbeat_writes", func(tx *gorm.DB) {
		if tx.Statement.Table != "settings" {
			writes++
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Callback().Update().Remove("test_node_heartbeat_writes") })
	usage = 5
	snap.Accounts[0].Remaining = 95
	snap.Accounts[0].ReportedUsage = 5
	if _, err := a.applySnapshot(snap, installed); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatal("meter-only heartbeat rewrote client tables", writes)
	}
	if service.GetNodeSetting("nodeSpeedUsage") != `{"cached-user":5}` {
		t.Fatal("cached snapshot did not update central period usage")
	}
	// Another location depleted the shared allowance; retain the raw local meter
	// and revoke locally even though the restart revision itself is unchanged.
	snap.Accounts[0].Remaining = 0
	if _, err := a.applySnapshot(snap, installed); err != nil {
		t.Fatal(err)
	}
	var traffic xray.ClientTraffic
	if err := db.Where("email = ?", "cached-user").First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Enable || traffic.Up != 5 || traffic.Total != 5 {
		t.Fatal("quota-only change was cached or reset meters", traffic)
	}
	// Reconnecting after clearing admission must reapply a fresh lease, even if
	// the panel sends the same desired state that preceded the disconnection.
	a.runtime = nil
	snap.Accounts[0].Remaining = 95
	if err := service.DisableNodeAdmission(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.applySnapshot(snap, installed); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("email = ?", "cached-user").First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if !traffic.Enable || traffic.Up != 5 {
		t.Fatal("fresh lease failed to restore admission with preserved meters")
	}
	before := snapshotApplyKey(snap, installed)
	snap.Files = map[string][]byte{"invalid": []byte("changed certificate")}
	if snapshotApplyKey(snap, installed) == before {
		t.Fatal("certificate changes bypass snapshot validation")
	}
}

func TestNodeReportAvoidsHostProbesAndKeepsLiveAcknowledgments(t *testing.T) {
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "probe-called")
	// Catalog preflight previously spawned systemctl repeatedly on every report.
	for _, name := range []string{"systemctl", "modinfo"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\ntouch '"+marker+"'\nexit 1\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	if err := service.SetNodeSetting(database.GetDB(), "nodeControlGeneration", "7"); err != nil {
		t.Fatal(err)
	}
	a := &Agent{diagnostics: nodeDiagnostics{Catalog: []service.CoreOption{{Name: "ipsec", Title: "IPsec"}}}}
	report, err := a.pendingReport()
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(report.CoreReport, &state); err != nil {
		t.Fatal(err)
	}
	if state["controlGeneration"] != "7" {
		t.Fatal("cached diagnostics hid a live acknowledgment", state)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("heartbeat ran a host probe", err)
	}
}
