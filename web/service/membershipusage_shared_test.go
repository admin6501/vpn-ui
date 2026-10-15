package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

// Reproduce the report: 71.71 GB total, but VMess shows 70.92 GB,
// Shadowsocks 71.46 GB, and AmneziaWG 260.26 MB. The first two contain
// the same unresolved pool; only their measured portions are additive.
func TestClientStatsForSeparatesSharedPoolFromMeasuredUsage(t *testing.T) {
	const email = "mycon"
	const pool = int64(72620 * mb)
	const poolUp = int64(120 * mb)
	shares := []membershipUsageRow{
		{InboundId: 1, Email: email, Protocol: model.VMESS},
		{InboundId: 2, Email: email, Protocol: model.Shadowsocks, Down: 554 * mb},
		{InboundId: 3, Email: email, Protocol: model.AWG, Down: 26026 * mb / 100},
	}
	total := &xray.ClientTraffic{Email: email, InboundId: 1, Enable: true, Up: poolUp, Down: pool - poolUp, AllTime: pool}
	split := &accountUsageSplit{
		byInbound: map[int]membershipUsageRow{}, hasMemberships: true, email: email,
		pooled: xray.ClientTraffic{Up: poolUp, Down: pool - poolUp, AllTime: pool},
	}
	for _, share := range shares {
		share.AllTime = share.Down
		split.byInbound[share.InboundId] = share
		total.Down += share.Down
		total.AllTime += share.AllTime
	}
	byEmail := map[string]*xray.ClientTraffic{email: total}
	splits := map[string]*accountUsageSplit{email: split}
	var service InboundService
	var measured, measuredAllTime int64
	for _, share := range shares {
		inbound := &model.Inbound{Id: share.InboundId, Protocol: share.Protocol}
		rows := service.clientStatsFor(inbound, byEmail, splits)
		if len(rows) != 1 {
			t.Fatalf("inbound %d: got %d rows, want 1", inbound.Id, len(rows))
		}
		row := rows[0]
		if row.Up != total.Up || row.Down != total.Down || row.AllTime != total.AllTime {
			t.Fatalf("inbound %d changed the authoritative account totals", inbound.Id)
		}
		wantShared := int64(0)
		if inbound.Id != 3 {
			wantShared = pool
		}
		if row.SharedUp+row.SharedDown != wantShared || row.SharedAllTime != wantShared {
			t.Errorf("inbound %d: shared = (%d, %d, %d), want current and lifetime totals %d",
				inbound.Id, row.SharedUp, row.SharedDown, row.SharedAllTime, wantShared)
		}
		if got := row.InboundDown - row.SharedDown; got != share.Down {
			t.Errorf("inbound %d: measured down = %d, want %d", inbound.Id, got, share.Down)
		}
		measured += row.InboundUp + row.InboundDown - row.SharedUp - row.SharedDown
		measuredAllTime += row.InboundAllTime - row.SharedAllTime
	}
	if measured+pool != total.Up+total.Down || measuredAllTime+pool != total.AllTime {
		t.Fatal("measured shares plus ONE shared pool must equal the account total")
	}
}
