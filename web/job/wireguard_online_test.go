package job

import "testing"

func TestWireguardHandshakeOnlineBeforeFirstTraffic(t *testing.T) {
	sessions := map[string]map[string]string{
		"wg-c": {"10.7.0.8/29": "MixedUser", "10.7.0.16/29": "MixedUser", "10.7.0.24/29": "MixedUser", "bad": "ignored"},
		"awg":  {"10.8.0.8/29": "MixedUser"}, "openvpn": {"10.2.0.2": "not-wireguard"},
	}
	ids := map[string]map[string]int{"wg-c": {"10.7.0.8/29": 10, "10.7.0.16/29": 10, "10.7.0.24/29": 11}, "awg": {"10.8.0.8/29": 12}}
	got := wireguardSessionOnline(sessions, ids)
	if len(got) != 3 {
		t.Fatalf("sessions should count once per membership: %+v", got)
	}
	seen := map[int]bool{}
	for _, record := range got {
		if !record.Online || record.Email != "MixedUser" || record.Up+record.Down != 0 || record.InboundId == 0 {
			t.Fatalf("invalid handshake evidence: %+v", record)
		}
		seen[record.InboundId] = true
	}
	if !seen[10] || !seen[11] || !seen[12] {
		t.Fatal(seen)
	}
	if len(wireguardSessionOnline(nil, nil)) != 0 {
		t.Fatal("removed sessions stayed online")
	}
}
