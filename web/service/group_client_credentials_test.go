package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestGroupCredentialValidationProtocolMatrix(t *testing.T) {
	newAccountsDB(t)
	vless := seedInboundWithClients(t, model.VLESS, 48101, []map[string]any{})
	names := []model.Protocol{model.VMESS, model.VLESS, model.TUIC, model.Shadowsocks, model.Trojan, model.ANYTLS, model.NAIVE, model.L2TP, model.PPTP, model.OPENVPN, model.OPENCONNECT, model.SSTP, model.IKEV2, model.SSH, model.Hysteria, model.Hysteria2, model.MTPROTO, model.WGC, model.AWG, model.WireGuard, model.GRE}
	inbounds := map[model.Protocol]*model.Inbound{}
	for i, p := range names {
		if p == model.VLESS {
			inbounds[p] = vless
			continue
		}
		inbounds[p] = seedInboundWithClients(t, p, 48200+i, []map[string]any{})
		if p == model.Shadowsocks {
			if err := database.GetDB().Model(inbounds[p]).Update("settings", `{"method":"chacha20-ietf-poly1305","clients":[]}`).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	base := map[string]any{"email": "sample", "id": "d0af1f64-5717-4562-b3fc-2c963f66afa6", "password": "safe-pass", "vpnUsername": "safe-user", "auth": "hy-secret", "secret": "0123456789abcdef0123456789abcdef", "enable": true}
	check := func(p model.Protocol, patch map[string]any, want string) {
		t.Helper()
		c := map[string]any{}
		for k, v := range base {
			c[k] = v
		}
		for k, v := range patch {
			c[k] = v
		}
		raw, _ := json.Marshal(map[string]any{"clients": []map[string]any{c}})
		ids := []int{vless.Id}
		if inbounds[p].Id != vless.Id {
			ids = append(ids, inbounds[p].Id)
		}
		err := ValidateGroupClientCredentials(ids, vless.Id, string(raw))
		if want == "" && err != nil {
			t.Errorf("%s valid: %v", p, err)
		}
		if want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Errorf("%s expected %q, got %v", p, want, err)
		}
	}
	for _, p := range names {
		t.Run(string(p), func(t *testing.T) { check(p, nil, "") })
	}
	for _, p := range []model.Protocol{model.VMESS, model.VLESS, model.TUIC} {
		t.Run(fmt.Sprintf("%s bad uuid", p), func(t *testing.T) {
			check(p, map[string]any{"id": "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"}, "client ID")
		})
	}
	for _, p := range []model.Protocol{model.Shadowsocks, model.Trojan, model.ANYTLS, model.NAIVE, model.TUIC, model.L2TP, model.PPTP, model.OPENVPN, model.OPENCONNECT, model.SSTP, model.IKEV2, model.SSH} {
		t.Run(fmt.Sprintf("%s missing password", p), func(t *testing.T) { check(p, map[string]any{"password": ""}, "password") })
	}
	for _, p := range []model.Protocol{model.L2TP, model.PPTP, model.OPENVPN, model.OPENCONNECT, model.SSTP, model.IKEV2, model.SSH} {
		t.Run(fmt.Sprintf("%s missing username", p), func(t *testing.T) { check(p, map[string]any{"vpnUsername": ""}, "VPN username") })
	}
	for _, p := range []model.Protocol{model.Hysteria, model.Hysteria2} {
		t.Run(fmt.Sprintf("%s missing auth", p), func(t *testing.T) { check(p, map[string]any{"auth": ""}, "auth") })
	}
	check(model.MTPROTO, map[string]any{"secret": "not-hex"}, "MTProto secret")
	for _, p := range []model.Protocol{model.WGC, model.AWG, model.WireGuard} {
		check(p, map[string]any{"privKey": "bad-key"}, "private key")
		check(p, map[string]any{"devices": []any{map[string]any{"privKey": ""}}}, "device private key")
	}
	key, _ := wgtypes.GeneratePrivateKey()
	other, _ := wgtypes.GeneratePrivateKey()
	check(model.WGC, map[string]any{"devices": []any{map[string]any{"privKey": key.String(), "pubKey": other.PublicKey().String()}}}, "public key")
	check(model.WGC, map[string]any{"devices": []any{map[string]any{"privKey": key.String(), "pubKey": key.PublicKey().String()}}}, "")
	check(model.GRE, map[string]any{"peers": []any{map[string]any{"peerIp": "invalid-ip"}}}, "peer IP")
	check(model.WireGuard, map[string]any{"addresses": []string{"invalid-address"}}, "address")
	check(model.VLESS, map[string]any{"id": "custom-id"}, "")
	check(model.TUIC, map[string]any{"id": "custom-id"}, "UUID")
	check(model.VLESS, map[string]any{"level": -1}, "client field type")
	check(model.VLESS, map[string]any{"encryption": false}, "client field type")
	check(model.VLESS, map[string]any{"flow": "invalid-flow"}, "flow")
	check(model.VLESS, map[string]any{"totalGB": -5}, "traffic quota")
	check(model.VLESS, map[string]any{"totalGB": "bad"}, "field type")
	// A selected secondary 2022 cipher must validate the supplied key too.
	target := inbounds[model.Shadowsocks]
	if err := database.GetDB().Model(target).Update("settings", fmt.Sprintf(`{"method":"2022-blake3-aes-256-gcm","password":%q,"clients":[]}`, base64.StdEncoding.EncodeToString(make([]byte, 32)))).Error; err != nil {
		t.Fatal(err)
	}
	check(model.Shadowsocks, nil, "base64")
	if err := database.GetDB().Model(target).Update("settings", `{"method":"2022-blake3-aes-256-gcm","password":"bad","clients":[]}`).Error; err != nil {
		t.Fatal(err)
	}
	check(model.Shadowsocks, nil, "server key")
	if err := database.GetDB().Model(target).Update("settings", fmt.Sprintf(`{"method":"2022-blake3-aes-256-gcm","password":%q,"clients":[]}`, base64.StdEncoding.EncodeToString(make([]byte, 32)))).Error; err != nil {
		t.Fatal(err)
	}
	client2022 := map[string]any{"email": "invalid-method", "password": base64.StdEncoding.EncodeToString(make([]byte, 32)), "method": "2022-blake3-aes-256-gcm"}
	badRaw, _ := json.Marshal(map[string]any{"clients": []map[string]any{client2022}})
	if err := ValidateGroupClientCredentials([]int{target.Id}, target.Id, string(badRaw)); err == nil || !strings.Contains(err.Error(), "per-client") {
		t.Errorf("2022 client method must be refused: %v", err)
	}
	if err := database.GetDB().Model(target).Update("settings", fmt.Sprintf(`{"method":"2022-blake3-chacha20-poly1305","password":%q,"clients":[]}`, base64.StdEncoding.EncodeToString(make([]byte, 32)))).Error; err != nil {
		t.Fatal(err)
	}
	delete(client2022, "method")
	client2022["level"] = 256
	badRaw, _ = json.Marshal(map[string]any{"clients": []map[string]any{client2022}})
	if err := ValidateGroupClientCredentials([]int{target.Id}, target.Id, string(badRaw)); err == nil || !strings.Contains(err.Error(), "field type") {
		t.Errorf("invalid Shadowsocks level must be refused: %v", err)
	}
	delete(client2022, "level")
	badRaw, _ = json.Marshal(map[string]any{"clients": []map[string]any{client2022}})
	if err := ValidateGroupClientCredentials([]int{target.Id}, target.Id, string(badRaw)); err == nil || !strings.Contains(err.Error(), "multi-user cipher") {
		t.Errorf("2022 chacha multi-user must be refused: %v", err)
	}
	if err := database.GetDB().Model(target).Update("settings", fmt.Sprintf(`{"method":"2022-blake3-aes-256-gcm","password":%q,"clients":[]}`, base64.StdEncoding.EncodeToString(make([]byte, 32)))).Error; err != nil {
		t.Fatal(err)
	}
	serverKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := database.GetDB().Model(target).Update("settings", fmt.Sprintf(`{"method":"2022-blake3-aes-256-gcm","password":%q,"clients":[{"address":"127.0.0.1","port":443}]}`, serverKey)).Error; err != nil {
		t.Fatal(err)
	}
	if err := ValidateGroupClientCredentials([]int{target.Id}, target.Id, string(badRaw)); err == nil || !strings.Contains(err.Error(), "relay address") {
		t.Errorf("missing relay address must be refused: %v", err)
	}
	client2022["address"] = "127.0.0.1"
	client2022["port"] = 443
	badRaw, _ = json.Marshal(map[string]any{"clients": []map[string]any{client2022}})
	if err := ValidateGroupClientCredentials([]int{target.Id}, target.Id, string(badRaw)); err != nil {
		t.Errorf("valid relay client was refused: %v", err)
	}
	if err := database.GetDB().Model(target).Update("settings", fmt.Sprintf(`{"method":"2022-blake3-aes-256-gcm","password":%q,"clients":[]}`, serverKey)).Error; err != nil {
		t.Fatal(err)
	}
	short := &model.Inbound{UserId: 1, Tag: "ss-128", Port: 48901, Protocol: model.Shadowsocks, Enable: true, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(short).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(short).Update("settings", fmt.Sprintf(`{"method":"2022-blake3-aes-128-gcm","password":%q,"clients":[]}`, base64.StdEncoding.EncodeToString(make([]byte, 16)))).Error; err != nil {
		t.Fatal(err)
	}
	client := map[string]any{"email": "multi-ss", "id": "d0af1f64-5717-4562-b3fc-2c963f66afa6", "password": base64.StdEncoding.EncodeToString(make([]byte, 16))}
	raw, _ := json.Marshal(map[string]any{"clients": []map[string]any{client}})
	if err := ValidateGroupClientCredentials([]int{vless.Id, short.Id, target.Id}, vless.Id, string(raw)); err != nil {
		t.Errorf("different 2022 ciphers should mint a secondary key: %v", err)
	}
}
