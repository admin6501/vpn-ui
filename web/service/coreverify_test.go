package service

import "testing"

func TestVerifiedProvisionedCores(t *testing.T) {
	present := func(kind, name string) bool { return name != "openvpn" && name != "ppp_generic" }
	ready, missing := verifiedProvisionedCores([]string{"openvpn", "pptp"}, nil, present)
	if len(ready) != 0 || len(missing) != 2 {
		t.Fatalf("missing dependencies recorded as installed: %v %v", ready, missing)
	}
	ready, missing = verifiedProvisionedCores([]string{"openvpn", "pptp"}, []string{"ppp_generic"}, present)
	if len(ready) != 1 || ready[0] != "pptp" || len(missing) != 1 || missing[0] != "openvpn" {
		t.Fatalf("reboot must not hide missing binaries: %v %v", ready, missing)
	}
}
