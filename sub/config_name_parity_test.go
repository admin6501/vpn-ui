package sub

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func configDisplayName(t *testing.T, link string) string {
	t.Helper()
	if strings.HasPrefix(link, "vmess://") {
		b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(link, "vmess://"))
		if err != nil {
			t.Fatal(err)
		}
		var value struct {
			PS string `json:"ps"`
		}
		if err = json.Unmarshal(b, &value); err != nil {
			t.Fatal(err)
		}
		return value.PS
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Fragment
}

// INFO, expanded-row copy and QR must retain the same configured name parts as
// the actual subscription generator, for every supported share-link protocol.
func TestConfigNamesMatchInfoCopyQRAndSubscription(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node required for browser-generator parity")
	}
	type fixture struct {
		Inbound     *model.Inbound `json:"inbound"`
		Email       string         `json:"email"`
		RemarkModel string         `json:"remarkModel"`
	}
	var fixtures []fixture
	for _, protocol := range []model.Protocol{model.VMESS, model.VLESS, model.Trojan, model.Shadowsocks, model.Hysteria, model.ANYTLS, model.TUIC, model.NAIVE} {
		for _, order := range []string{"-ieo", "|ioe", "_oei", "-eo"} {
			for _, endpoints := range []string{`[]`, `[{"dest":"edge.example.com","port":8443,"remark":"لبه A","forceTls":"same"},{"dest":"edge2.example.com","port":9443,"remark":"لبه B","forceTls":"same"}]`} {
				fixtures = append(fixtures, fixture{Inbound: &model.Inbound{Id: 1, Tag: "inbound-443", Remark: "سرور اصلی", Port: 443, Listen: "vpn.example.com", Protocol: protocol,
					Settings:       `{"clients":[{"email":"alice","id":"11111111-2222-3333-4444-555555555555","password":"password","auth":"password","username":"alice-login","enable":true}],"decryption":"none","method":"aes-128-gcm","password":"server-password","version":2,"network":"tcp"}`,
					StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.com"},"externalProxy":` + endpoints + `}`}, Email: "alice", RemarkModel: order})
			}
		}
	}
	input, err := json.Marshal(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "test_unit/config_name_parity.cjs")
	cmd.Dir = ".."
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser generators: %v\n%s", err, output)
	}
	var results []struct{ Info, Credentials, QR []string }
	if err = json.Unmarshal(output, &results); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	if len(results) != len(fixtures) {
		t.Fatalf("got %d results, want %d", len(results), len(fixtures))
	}
	for i, f := range fixtures {
		s := &SubService{address: "vpn.example.com", remarkModel: f.RemarkModel}
		subscription := strings.Split(strings.TrimSpace(s.getLink(f.Inbound, f.Email)), "\n")
		for surface, links := range map[string][]string{"INFO": results[i].Info, "copy": results[i].Credentials, "QR": results[i].QR} {
			if len(links) != len(subscription) {
				t.Errorf("%s %s %s: got %d endpoints want %d", f.Inbound.Protocol, f.RemarkModel, surface, len(links), len(subscription))
				continue
			}
			for j, link := range links {
				got, want := configDisplayName(t, link), configDisplayName(t, subscription[j])
				if got != want {
					t.Errorf("%s %s %s endpoint %d: got %q want %q", f.Inbound.Protocol, f.RemarkModel, surface, j, got, want)
				}
			}
		}
	}
}
