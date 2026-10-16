package xray

import (
	"encoding/base64"
	"testing"

	statsService "github.com/xtls/xray-core/app/stats/command"
)

func membershipStat(tag, email, direction string, value int64) *statsService.Stat {
	enc := base64.RawURLEncoding.EncodeToString
	return &statsService.Stat{Name: "membership>>>" + enc([]byte(tag)) + "." + enc([]byte(email)) + ">>>traffic>>>" + direction, Value: value}
}

func TestMembershipTrafficUsesDetailedCountersOnce(t *testing.T) {
	input := []*statsService.Stat{
		{Name: "user>>>same>>>traffic>>>uplink", Value: 100},
		{Name: "user>>>same>>>traffic>>>downlink", Value: 200},
		membershipStat("vmess", "same", "uplink", 30), membershipStat("vmess", "same", "downlink", 40),
		membershipStat("ss", "same", "uplink", 70), membershipStat("ss", "same", "downlink", 160),
		{Name: "user>>>legacy>>>traffic>>>downlink", Value: 9},
	}
	_, records := parseTrafficStats(input)
	totals := map[string]int64{}
	for _, r := range records {
		totals[r.Email] += r.Up + r.Down
		if r.Email == "same" && (!r.SourceTagged || r.InboundTag == "") {
			t.Fatal("detailed counter lost its source")
		}
	}
	if totals["same"] != 300 || totals["legacy"] != 9 || len(records) != 3 {
		t.Fatalf("duplicate/lost traffic: totals=%v, rows=%+v", totals, records)
	}
}

// QueryStats resets individual counters while writers continue. The legacy
// total can be ahead or behind the detailed snapshot. Never bill the difference.
func TestMembershipTrafficDoesNotBillLegacyResetSkew(t *testing.T) {
	var charged int64
	for _, tick := range []struct{ legacy, detail int64 }{{170, 150}, {0, 20}, {20, 0}} {
		_, rows := parseTrafficStats([]*statsService.Stat{
			{Name: "user>>>same>>>traffic>>>uplink", Value: tick.legacy},
			{Name: "user>>>same>>>traffic>>>downlink", Value: 0},
			membershipStat("vmess", "same", "uplink", tick.detail), membershipStat("vmess", "same", "downlink", 0),
		})
		if len(rows) != 1 {
			t.Fatalf("legacy duplicate remains in zero tick: %+v", rows)
		}
		charged += rows[0].Up + rows[0].Down
	}
	if charged != 170 {
		t.Fatalf("charged %d, want 170", charged)
	}
}

func TestMembershipTrafficFallbackByDirectionAndMalformedInput(t *testing.T) {
	_, rows := parseTrafficStats([]*statsService.Stat{
		{Name: "user>>>same>>>traffic>>>uplink", Value: 100},
		{Name: "user>>>same>>>traffic>>>downlink", Value: 50},
		membershipStat("ss", "same", "uplink", 100),
		{Name: "membership>>>_.c2FtZQ>>>traffic>>>downlink", Value: 99},
		{Name: "prefixuser>>>same>>>traffic>>>downlink", Value: 999}, nil,
	})
	var up, down int64
	for _, r := range rows {
		up += r.Up
		down += r.Down
	}
	if up != 100 || down != 50 {
		t.Fatalf("fallback lost or duplicated bytes: %d/%d", up, down)
	}
}

func TestMembershipTrafficEscapesIdentityAndPreservesUnknownTags(t *testing.T) {
	const email = "کاربر.name+alias@example.com"
	_, rows := parseTrafficStats([]*statsService.Stat{membershipStat("tag>>>with.dot", email, "downlink", 7), membershipStat("", email, "downlink", 3)})
	if len(rows) != 2 {
		t.Fatalf("cannot decode exact identities: %+v", rows)
	}
	for _, r := range rows {
		if r.Email != email || !r.SourceTagged {
			t.Fatalf("changed identity: %+v", r)
		}
	}
}
