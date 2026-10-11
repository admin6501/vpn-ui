package sub

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"net/url"
	"strings"
	"testing"
)

func TestHysteriaCredentialAndIPv6Links(t *testing.T) {
	s := &SubService{address: "2001:db8::1", remarkModel: "-ieo"}
	in := &model.Inbound{Port: 443, Protocol: model.Hysteria, Settings: `{"clients":[{"email":"alice","auth":"p@ss:w/rd?x#y%"}]}`, StreamSettings: `{"tlsSettings":{"alpn":["h3",123],"settings":{"allowInsecure":"bad"}}}`}
	for _, external := range []bool{false, true} {
		if external {
			in.StreamSettings = `{"externalProxy":[{"dest":"2001:db8::2","port":8443}],"tlsSettings":{}}`
		}
		link := s.genHysteriaLink(in, "alice")
		u, err := url.Parse(link)
		if err != nil || u.User == nil || u.User.Username() != "p@ss:w/rd?x#y%" || !strings.Contains(u.Hostname(), ":") {
			t.Fatalf("broken link: %q %v", link, err)
		}
	}
	if s.genHysteriaLink(in, "missing") != "" {
		t.Fatal("missing client generated link")
	}
	in.StreamSettings = "{"
	if s.genHysteriaLink(in, "alice") != "" {
		t.Fatal("malformed stream accepted")
	}
}
func TestHysteriaJSONMissingTransportAndLegacyProtocol(t *testing.T) {
	s := &SubJsonService{}
	in := &model.Inbound{Port: 443, Listen: "vpn.example", Protocol: model.Protocol("hysteria2"), Settings: `{}`, StreamSettings: `{"finalmask":{"udp":[{"type":"salamander","settings":{"password":"obfs"}}]}}`}
	raw := s.genHy(in, map[string]any{}, model.Client{Auth: "secret"})
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["protocol"] != "hysteria" {
		t.Fatal(out)
	}
	st := out["streamSettings"].(map[string]any)
	hy := st["hysteriaSettings"].(map[string]any)
	if hy["version"] != float64(2) || hy["auth"] != "secret" || st["finalmask"] == nil {
		t.Fatal(st)
	}
}

func TestHysteriaExternalProxyProfilesRemainIndependent(t *testing.T) {
	s := &SubJsonService{SubService: &SubService{remarkModel: "-ieo"}}
	in := &model.Inbound{Port: 443, Listen: "original", Protocol: model.Hysteria, Settings: `{"version":2}`, StreamSettings: `{"security":"tls","externalProxy":[null,{"dest":"one.example","port":8443,"forceTls":"none"},{"dest":"two.example","port":9443}],"finalmask":{"udp":[{"type":"salamander","settings":{"password":"obfs"}}]}}`}
	configs := s.getConfig(in, model.Client{Email: "alice", Auth: "secret"}, "vpn.example")
	if len(configs) != 2 || in.Listen != "original" || in.Port != 443 {
		t.Fatalf("input mutated or profiles missing: %v %d", in, len(configs))
	}
	for i, raw := range configs {
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatal(err)
		}
		outs := cfg["outbounds"].([]any)
		out := outs[0].(map[string]any)
		settings := out["settings"].(map[string]any)
		st := out["streamSettings"].(map[string]any)
		if settings["address"] != []string{"one.example", "two.example"}[i] || st["security"] != "tls" || st["finalmask"] == nil {
			t.Fatal(out)
		}
	}
}
