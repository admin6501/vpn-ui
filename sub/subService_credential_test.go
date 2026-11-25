package sub

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestNativeProtocolsStayOutOfRawSubscription(t *testing.T) {
	for _, protocol := range []model.Protocol{model.OPENVPN, model.OPENCONNECT, model.L2TP, model.PPTP,
		model.IKEV2, model.SSTP, model.SSH, model.MTPROTO, model.AWG, model.GRE} {
		t.Run(string(protocol), func(t *testing.T) {
			service := &SubService{address: "vpn.example.com"}
			inbound := &model.Inbound{Protocol: protocol, Port: 443, Remark: "London"}
			client := model.Client{ID: "vpn-login", Email: "subscriber", Password: "p p", Secret: "secret"}
			if got := service.getLink(inbound, client.Email); got != "" {
				t.Fatalf("native protocol escaped into raw subscription: %q", got)
			}
			card, ok := service.nativeConnection(inbound, client)
			if !ok || card.Name != "London" || card.Host != "vpn.example.com" {
				t.Fatalf("native card: %+v", card)
			}
			if protocol == model.SSH && (card.Username != "vpn-login" || card.Password != "p p") {
				t.Fatal("SSH must show the VPN login, not the account email")
			}
			if protocol == model.MTPROTO && (card.Secret != "secret" || card.Password != "") {
				t.Fatal("MTProto must only show its secret")
			}
			if protocol == model.GRE && card.Port != 0 {
				t.Fatal("GRE must not display its synthetic inbound port")
			}
		})
	}
}

func TestNativePSKVisibility(t *testing.T) {
	service := &SubService{address: "vpn.example.com"}
	for _, test := range []struct {
		protocol model.Protocol
		settings string
		want     string
	}{
		{model.L2TP, `{"ipsecEnable":true,"ipsecPsk":"key"}`, "key"},
		{model.L2TP, `{"ipsecEnable":false,"ipsecPsk":"stale"}`, ""},
		{model.IKEV2, `{"authMode":"psk","psk":"key"}`, "key"},
		{model.IKEV2, `{"authMode":"eap-mschapv2","psk":"stale"}`, ""},
	} {
		card, ok := service.nativeConnection(&model.Inbound{Protocol: test.protocol, Settings: test.settings}, model.Client{})
		if !ok || card.PSK != test.want {
			t.Fatalf("%s settings %s produced wrong PSK", test.protocol, test.settings)
		}
	}
}

func TestNativeSSHCardUsesAdvertisedEndpoints(t *testing.T) {
	s := &SubService{address: "panel.example.com"}
	inbound := &model.Inbound{Protocol: model.SSH, Port: 22, Settings: `{
		"clients":[{"email":"client","id":"login","password":"pw"}],
		"externalProxy":[{"dest":"one.example.com","port":2222},{"dest":"two.example.com","port":2223}]
	}`}
	card, ok := s.nativeConnection(inbound, model.Client{Email: "client", ID: "login", Password: "pw"})
	if !ok || card.Host != "one.example.com" || card.Port != 2222 ||
		len(card.OtherEndpoints) != 1 || card.OtherEndpoints[0] != "two.example.com:2223" {
		t.Fatalf("unexpected advertised endpoints: %+v", card)
	}
}

func TestNativeVPNCardsUseLoginAndExternalProxy(t *testing.T) {
	s := &SubService{address: "panel.example.com"}
	for _, protocol := range []model.Protocol{model.L2TP, model.PPTP, model.IKEV2, model.OPENVPN, model.OPENCONNECT, model.SSTP} {
		t.Run(string(protocol), func(t *testing.T) {
			inbound := &model.Inbound{Protocol: protocol, Port: 1443, Settings: `{
				"authMode":"eap-mschapv2",
				"externalProxy":[{"dest":" "},{"dest":" edge.example.com ","port":8443},{"dest":"2001:db8::2"}]
			}`}
			client := model.Client{Email: "account-label", ID: "actual-vpn-login", Password: "pw"}
			card, ok := s.nativeConnection(inbound, client)
			if !ok || card.Username != client.ID || card.Password != client.Password ||
				card.Host != "edge.example.com" || card.Port != 8443 ||
				len(card.OtherEndpoints) != 1 || card.OtherEndpoints[0] != "[2001:db8::2]:1443" {
				t.Fatalf("wrong card: %+v", card)
			}
			inbound.Settings = `{"externalProxy":[{"dest":""}]}`
			client.ID = ""
			card, ok = s.nativeConnection(inbound, client)
			if !ok || card.Username != client.Email || card.Host != "panel.example.com" || card.Port != 1443 {
				t.Fatalf("wrong legacy fallback: %+v", card)
			}
		})
	}
}

func TestIKEv2CertificateAndPSKModesDoNotExposeUnusedLogin(t *testing.T) {
	s := &SubService{address: "panel.example.com"}
	for _, mode := range []string{"psk", "eap-tls"} {
		inbound := &model.Inbound{Protocol: model.IKEV2, Settings: `{"authMode":"` + mode + `","psk":"shared-key"}`}
		card, ok := s.nativeConnection(inbound, model.Client{Email: "account", ID: "login", Password: "pw"})
		if !ok || card.Username != "" || card.Password != "" {
			t.Fatalf("%s showed unused login credentials: %+v", mode, card)
		}
	}
}
