package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestNodeSnapshotHistoryAcceptsFinalRemovedAssignmentWithoutResettingMeters(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	in, a := seedNodeAccount(t, n, 49301, "historical@example.com")
	if _, err := svc.Snapshot(n.ID); err != nil {
		t.Fatal(err)
	}
	report := nodewire.Report{Version: nodewire.Version, Sequence: 1, Counters: []nodewire.Counter{{Kind: "account", AccountID: a.Id, Up: 40}, {Kind: "membership", AccountID: a.Id, InboundID: in.Id, Up: 40}, {Kind: "inbound", InboundID: in.Id, Up: 40}}}
	if err := svc.AcceptReport(n.ID, report); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Snapshot(n.ID); err != nil {
		t.Fatal(err)
	}
	var cursor model.NodeCounter
	if err := database.GetDB().Where("node_id = ? AND kind = ? AND account_id = ?", n.ID, "account", a.Id).First(&cursor).Error; err != nil || cursor.Up != 40 {
		t.Fatal("snapshot reset accepted counter", err, cursor.Up)
	}
	database.GetDB().Where("inbound_id = ?", in.Id).Delete(&model.AccountInbound{})
	database.GetDB().Delete(&model.Inbound{}, in.Id)
	report.Sequence = 2
	for i := range report.Counters {
		report.Counters[i].Up = 70
	}
	if err := svc.AcceptReport(n.ID, report); err != nil {
		t.Fatal("final scoped history rejected", err)
	}
	if err := svc.AcceptReport(n.ID, report); err != nil {
		t.Fatal("retry rejected", err)
	}
	var traffic xray.ClientTraffic
	database.GetDB().Where("email = ?", a.Email).First(&traffic)
	if traffic.Up != 70 || traffic.AllTime != 70 {
		t.Fatal("final meter lost or billed twice", traffic.Up, traffic.AllTime)
	}
}

func TestNodeSnapshotHistoryRecordsScopeBeforeFirstReport(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	in, a := seedNodeAccount(t, n, 49302, "removed-before-report@example.com")
	if _, err := svc.Snapshot(n.ID); err != nil {
		t.Fatal(err)
	}
	database.GetDB().Where("inbound_id = ?", in.Id).Delete(&model.AccountInbound{})
	database.GetDB().Delete(&model.Inbound{}, in.Id)
	report := nodewire.Report{Version: nodewire.Version, Sequence: 1, Counters: []nodewire.Counter{{Kind: "account", AccountID: a.Id, Up: 5}, {Kind: "membership", AccountID: a.Id, InboundID: in.Id, Up: 5}, {Kind: "inbound", InboundID: in.Id, Up: 5}}}
	if err := svc.AcceptReport(n.ID, report); err != nil {
		t.Fatal("assignment removed before first report permanently blocked sync", err)
	}
}

func TestNodeLegacyZeroTombstonesDoNotAuthorizeForeignCounters(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	seedNodeAccount(t, n, 49303, "own-zero@example.com")
	other, _, err := svc.Create("Other", "other.example.com")
	if err != nil {
		t.Fatal(err)
	}
	foreignInbound, foreign := seedNodeAccount(t, other, 49304, "foreign-zero@example.com")
	if _, err := svc.Snapshot(n.ID); err != nil {
		t.Fatal(err)
	}
	report := nodewire.Report{Version: nodewire.Version, Sequence: 1, Counters: []nodewire.Counter{{Kind: "account", AccountID: foreign.Id}, {Kind: "membership", AccountID: foreign.Id, InboundID: foreignInbound.Id}, {Kind: "inbound", InboundID: foreignInbound.Id}}}
	if err := svc.AcceptReport(n.ID, report); err != nil {
		t.Fatal("zero legacy tombstone blocked sync", err)
	}
	var count int64
	database.GetDB().Model(&model.NodeCounter{}).Where("node_id = ? AND (account_id = ? OR inbound_id = ?)", n.ID, foreign.Id, foreignInbound.Id).Count(&count)
	if count != 0 {
		t.Fatal("ignored zero record granted authorization to foreign identity")
	}
	report.Sequence = 2
	report.Counters[0].Up = 1
	err = svc.AcceptReport(n.ID, report)
	if err == nil || NodeReportConflictCode(err) != "counter_out_of_scope" {
		t.Fatal("positive foreign usage was accepted", err)
	}
	var traffic xray.ClientTraffic
	database.GetDB().Where("email = ?", foreign.Email).First(&traffic)
	if traffic.Up != 0 || traffic.Down != 0 {
		t.Fatal("foreign bill modified")
	}
}
