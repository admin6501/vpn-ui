package service

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"testing"
)

func TestWorkerMultiDeletePreservesSharedTrafficUntilLastInbound(t *testing.T) {
	svc := newInboundDB(t)
	shared := testClient("multi-bot@example.com")
	first := seedTaggedInbound(t, model.VLESS, 49401, []map[string]any{shared})
	second := seedTaggedInbound(t, model.VLESS, 49402, []map[string]any{shared})
	seedTrafficRow(t, xray.ClientTraffic{InboundId: first.Id, Email: "multi-bot@example.com", Up: 111, Down: 222, AllTime: 333, Enable: true})
	if err := database.GetDB().Create(&model.InboundClientIps{ClientEmail: "multi-bot@example.com", Ips: "[\"127.0.0.1\"]"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DelInboundClientByEmail(first.Id, "multi-bot@example.com"); err != nil {
		t.Fatal(err)
	}
	row := trafficRow(t, "multi-bot@example.com")
	if row == nil || row.Up != 111 || row.Down != 222 || row.AllTime != 333 || row.InboundId != second.Id {
		t.Fatalf("shared traffic lost or not moved: %+v", row)
	}
	var n int64
	database.GetDB().Model(&model.InboundClientIps{}).Count(&n)
	if n != 1 {
		t.Fatal("shared IP binding lost")
	}
	if _, err := svc.DelInboundClientByEmail(second.Id, "multi-bot@example.com"); err != nil {
		t.Fatal(err)
	}
	if trafficRow(t, "multi-bot@example.com") != nil {
		t.Fatal("final deletion left traffic")
	}
	database.GetDB().Model(&model.InboundClientIps{}).Count(&n)
	if n != 0 {
		t.Fatal("final deletion left IP binding")
	}
}

// Each API call takes its own snapshot; the final deletion must still see the
// actual consumed traffic so that the reseller's refund is computed once.
func TestMultiDeleteRetainsResellerUsageForFinalRefund(t *testing.T) {
	f := newMembershipFixture(t, model.ResellerProfile{AllowanceBytes: 100 * gb, SpentBytes: 40 * gb}, 10*gb)
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "sold-one").Update("all_time", 3*gb).Error; err != nil {
		t.Fatal(err)
	}
	for _, inbound := range []*model.Inbound{f.home, f.away} {
		used, known, err := f.rs.UsageOf("sold-one")
		if err != nil || !known || used != 3*gb {
			t.Fatalf("usage before deleting inbound %d: %d, %v, %v", inbound.Id, used, known, err)
		}
		if _, err := f.svc.DelInboundClientByEmail(inbound.Id, "sold-one"); err != nil {
			t.Fatal(err)
		}
		if err := f.rs.RefundDeleted("sold-one", used, known); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.spent(t); got != 33*gb {
		t.Fatalf("spent=%d, want %d", got, 33*gb)
	}
}
