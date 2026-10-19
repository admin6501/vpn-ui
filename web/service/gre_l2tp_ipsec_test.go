package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestGreAndL2tpUseSeparateIKEVersions(t *testing.T) {
	l2tp := &model.Inbound{Id: 1, Protocol: model.L2TP, Enable: true, Settings: `{"ipsecEnable":true,"ipsecPsk":"l2tp-lab-secret"}`}
	gre := &model.Inbound{Id: 7, Protocol: model.GRE, Enable: true}
	files := map[string]string{
		"l2tp.conf":  (&L2tpService{}).buildL2tpSwanctlConn([]*model.Inbound{l2tp}),
		"gre-7.conf": (&GreService{}).buildGreSwanctlConn(gre, &greSettings{IpsecEnable: true, IpsecPsk: "gre-lab-secret"}),
	}
	// IKEv1 chooses the authentication config before the UDP/GRE selectors are
	// available. Keeping GRE out of that negotiation is the coexistence contract.
	if !strings.Contains(files["l2tp.conf"], "version = 1\n") || !strings.Contains(files["l2tp.conf"], "dynamic[udp/1701]") {
		t.Fatal("native L2TP/IPsec negotiation changed")
	}
	if !strings.Contains(files["gre-7.conf"], "version = 2\n") || strings.Contains(files["gre-7.conf"], "version = 0\n") {
		t.Fatal("GRE can capture L2TP's IKEv1 authentication on the shared daemon")
	}
	if !strings.Contains(files["gre-7.conf"], "dynamic[gre]") || !strings.Contains(files["gre-7.conf"], "id_local = gre-7.vpn-ui") {
		t.Fatal("GRE lost its own transport selectors or identity")
	}
	// Optional export feeds the real two-client strongSwan negotiation check.
	if dir := os.Getenv("IPSEC_FIXTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for name, text := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
