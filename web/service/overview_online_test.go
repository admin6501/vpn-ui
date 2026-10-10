package service

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"strconv"
	"testing"
)

func TestOverviewCountsOnlineAccountOnceAcrossFiveInbounds(t *testing.T) {
	svc := newAccountsDB(t)
	const email = "single-online@example.com"
	first, second := seedTwoInboundAccount(t, svc, email)
	db := database.GetDB()
	ids := []int{first.Id, second.Id}
	for i := 0; i < 3; i++ {
		inbound := *second
		inbound.Id = 0
		inbound.Tag = "overview-test-" + strconv.Itoa(i)
		inbound.Port = 24000 + i
		if err := db.Create(&inbound).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, inbound.Id)
	}
	if _, err := svc.ApplyMemberships(email, ids, nil, true); err != nil {
		t.Fatal(err)
	}
	inbounds := &InboundService{}
	err, _, _, _, _ := inbounds.AddTraffic(nil, []*xray.ClientTraffic{{InboundId: first.Id, Email: email, Down: mb}})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := inbounds.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 1 {
		t.Fatalf("one account on five inbounds must count once in Total, got %d", stats.Total)
	}
	if stats.Online != 1 {
		t.Fatalf("one account on five inbounds counted as %d online", stats.Online)
	}
	// A second active membership is still the same online customer.
	err, _, _, _, _ = inbounds.AddTraffic(nil, []*xray.ClientTraffic{{InboundId: first.Id, Email: email, Down: mb}, {InboundId: second.Id, Email: email, Down: mb}})
	if err != nil {
		t.Fatal(err)
	}
	stats, err = inbounds.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || stats.Online != 1 {
		t.Fatalf("multiple active memberships duplicated account: %+v %v", stats, err)
	}
}
