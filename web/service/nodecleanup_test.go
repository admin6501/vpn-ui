package service

import "testing"

func TestNodeCleanupRequiresDedicatedMode(t *testing.T) {
	t.Setenv("VPNUI_NODE_AGENT", "")
	report, err := CleanupNodeHost()
	if err == nil || report != nil {
		t.Fatal("central panel process allowed node cleanup")
	}
}

func TestNodeCleanupSelectionIgnoresUnclaimedHostCores(t *testing.T) {
	names := nodeCleanupSelection([]string{"openvpn"}, []OwnedArtifactReport{{Cores: []string{"openvpn", "gre"}}, {Cores: []string{"unknown", "xray"}}})
	if len(names) != 2 {
		t.Fatalf("unexpected selection: %v", names)
	}
	if nodeCleanupHasTunnelCore([]string{"ssh", "mtproto", "xray"}) {
		t.Fatal("relay cleanup must not delete the host's tunnel network")
	}
	if !nodeCleanupHasTunnelCore(names) {
		t.Fatal("native VPN network cleanup skipped")
	}
	if len(nodeCleanupSelection(nil, nil)) != 0 {
		t.Fatal("empty node inferred host cores from OS state")
	}
}
