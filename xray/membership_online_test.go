package xray

import (
	"encoding/base64"
	"testing"
)

func TestMembershipOnlineRecordsDecodeWithoutBilling(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	rows := membershipOnlineRecords(map[string]int64{enc("tuic-1") + "." + enc("Mixed+user@example.com"): 1, "malformed": 1, "!bad.bad": 1})
	if len(rows) != 1 || rows[0].Email != "Mixed+user@example.com" || rows[0].InboundTag != "tuic-1" || !rows[0].Online || rows[0].Up+rows[0].Down != 0 {
		t.Fatalf("invalid online snapshot: %+v", rows)
	}
}
