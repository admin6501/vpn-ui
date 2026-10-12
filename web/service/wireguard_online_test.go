package service

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"testing"
)

func TestWireguardHandshakeOnlineWithoutBillableTraffic(t *testing.T) {
	accounts := newAccountsDB(t)
	const email = "handshake-only@example.com"
	first, second := seedTwoInboundAccount(t, accounts, email)
	svc := &InboundService{}
	err, _, _, _, _ := svc.AddTraffic(nil, []*xray.ClientTraffic{{Email: email, InboundId: first.Id, Online: true}, {Email: email, InboundId: first.Id, Online: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !equal(svc.GetOnlineMemberships(), []string{onlineMembershipKey(first.Id, email)}) {
		t.Fatal(svc.GetOnlineMemberships())
	}
	summary, err := svc.GetClientStatsFor(&model.User{Enable: true, IsSuperAdmin: true})
	if err != nil || summary.Online != 1 {
		t.Fatal(summary, err)
	}
	var usage xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", email).First(&usage).Error; err != nil {
		t.Fatal(err)
	}
	if usage.Up+usage.Down != 0 || usage.AllTime != 0 || usage.LastOnline == 0 {
		t.Fatalf("handshake lost or billed: %+v", usage)
	}
	for _, pair := range svc.GetOnlineMemberships() {
		if pair == onlineMembershipKey(second.Id, email) {
			t.Fatal("unrelated inbound is online")
		}
	}
	svc.ClearOnlineStatus()
	if len(svc.GetOnlineClients()) != 0 {
		t.Fatal("cleared session retained")
	}
}
