package service

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	xrayuuid "github.com/xtls/xray-core/common/uuid"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// ValidateGroupClientCredentials checks all explicitly selected inbounds before
// creating an anchor client or charging a representative. Never log credentials.
func ValidateGroupClientCredentials(inboundIDs []int, anchorID int, settings string) error {
	var payload struct {
		Clients []map[string]any `json:"clients"`
	}
	if err := json.Unmarshal([]byte(settings), &payload); err != nil {
		return fmt.Errorf("invalid client settings: %w", err)
	}
	if len(payload.Clients) == 0 {
		return fmt.Errorf("at least one client is required")
	}
	var typed struct {
		Clients []model.Client `json:"clients"`
	}
	if err := json.Unmarshal([]byte(settings), &typed); err != nil {
		return fmt.Errorf("invalid client field type: %w", err)
	}
	if err := validateClientLimits(typed.Clients); err != nil {
		return err
	}
	for _, client := range typed.Clients {
		if err := ValidateClientEmail(strings.TrimSpace(client.Email)); err != nil {
			return err
		}
		if err := ValidateClientSubID(client.SubID); err != nil {
			return err
		}
		if client.TotalGB < 0 || client.Reset < 0 {
			return fmt.Errorf("invalid traffic quota or reset interval")
		}
	}
	var inbounds []model.Inbound
	if len(inboundIDs) == 0 {
		return nil
	}
	if err := database.GetDB().Where("id IN ?", inboundIDs).Find(&inbounds).Error; err != nil {
		return err
	}
	if len(inbounds) != len(inboundIDs) {
		return fmt.Errorf("one or more selected inbounds were not found")
	}
	var anchor model.Protocol
	for _, in := range inbounds {
		switch in.Protocol {
		case model.VMESS, model.VLESS, model.TUIC, model.Shadowsocks, model.Trojan, model.ANYTLS, model.NAIVE, model.L2TP, model.PPTP, model.OPENVPN, model.OPENCONNECT, model.SSTP, model.IKEV2, model.SSH, model.Hysteria, model.Hysteria2, model.MTPROTO, model.WGC, model.AWG, model.WireGuard, model.GRE:
		default:
			return fmt.Errorf("inbound %d (%s) does not support managed group clients", in.Id, in.Protocol)
		}
		if !settingsHoldClients(in.Protocol, in.Settings) {
			return fmt.Errorf("inbound %d (%s) has no managed client list", in.Id, in.Protocol)
		}
		if in.Id == anchorID {
			anchor = in.Protocol
			break
		}
	}
	stringField := func(m map[string]any, key string) string { v, _ := m[key].(string); return v }
	for _, c := range payload.Clients {
		for _, in := range inbounds {
			fail := func(field string) error {
				return fmt.Errorf("inbound %q (id %d, %s): invalid or missing %s", in.Remark, in.Id, in.Protocol, field)
			}
			if in.Protocol == model.VLESS || in.Protocol == model.VMESS || in.Protocol == model.Shadowsocks || in.Protocol == model.Trojan || in.Protocol == model.ANYTLS || in.Protocol == model.TUIC || in.Protocol == model.NAIVE {
				// Xray parses these optional fields outside model.Client. Reject wrong
				// JSON types and integer bounds before they reach its configuration.
				raw, err := json.Marshal(c)
				if err != nil {
					return fail("client fields")
				}
				var extra struct {
					Level      uint32   `json:"level"`
					Encryption string   `json:"encryption"`
					Testseed   []uint32 `json:"testseed"`
				}
				if err := json.Unmarshal(raw, &extra); err != nil {
					return fail("client field type")
				}
			}
			password := stringField(c, "password")
			validText := func(s string) bool {
				if strings.TrimSpace(s) == "" {
					return false
				}
				for _, r := range s {
					if unicode.IsControl(r) || r == unicode.ReplacementChar {
						return false
					}
				}
				return true
			}
			id := stringField(c, "uuid")
			if anchor == model.VMESS || anchor == model.VLESS || anchor == model.TUIC || (anchorID == 0 && stringField(c, "id") != "") {
				id = stringField(c, "id")
			}
			switch in.Protocol {
			case model.VMESS, model.VLESS:
				if strings.TrimSpace(id) != id {
					return fail("client ID")
				}
				if _, err := xrayuuid.ParseString(id); err != nil {
					return fail("client ID")
				}
			case model.TUIC:
				if _, err := uuid.Parse(id); err != nil {
					return fail("UUID")
				}
			}
			switch in.Protocol {
			case model.Trojan, model.Shadowsocks, model.ANYTLS, model.NAIVE, model.TUIC, model.L2TP, model.PPTP, model.OPENVPN, model.OPENCONNECT, model.SSTP, model.IKEV2, model.SSH:
				if !validText(password) {
					return fail("password")
				}
			}
			if in.Protocol == model.Shadowsocks {
				var clientSS struct {
					Level   uint8   `json:"level"`
					Address *string `json:"address"`
					Port    uint16  `json:"port"`
				}
				if in.Id == anchorID {
					raw, err := json.Marshal(c)
					if err != nil || json.Unmarshal(raw, &clientSS) != nil {
						return fail("Shadowsocks client field type")
					}
				}
				method := ""
				if in.Id == anchorID {
					method = stringField(c, "method")
				}
				parentMethod := strings.ToLower(inboundMethod(in.Settings))
				if size := shadowsocksMethodKeySize(parentMethod); size > 0 {
					// Xray 2022 multi-user takes the server cipher, requires an
					// empty per-client method and does not support chacha20 here.
					if !strings.Contains(parentMethod, "aes") {
						return fail("2022 multi-user cipher method")
					}
					if method != "" {
						return fail("2022 per-client cipher method (must be empty)")
					}
					var server struct {
						Password string `json:"password"`
						Clients  []struct {
							Address *string `json:"address"`
						} `json:"clients"`
					}
					if json.Unmarshal([]byte(in.Settings), &server) != nil || !validShadowsocksKey(server.Password, size) {
						return fail("2022 server key")
					}
					relay := len(server.Clients) > 0 && server.Clients[0].Address != nil
					if relay && in.Id != anchorID {
						return fail("2022 relay requires an explicitly addressed inbound")
					}
					if len(server.Clients) == 0 && clientSS.Address != nil {
						relay = true
					}
					if relay {
						if clientSS.Address == nil || strings.TrimSpace(*clientSS.Address) == "" {
							return fail("2022 relay address")
						}
						if clientSS.Port == 0 {
							return fail("2022 relay port")
						}
					} else if in.Id == anchorID && clientSS.Address != nil {
						return fail("2022 mixed relay address")
					}
				}
				effectiveMethod := method
				if effectiveMethod == "" {
					effectiveMethod = parentMethod
				}
				{
					switch strings.ToLower(effectiveMethod) {
					case "aes-128-gcm", "aead_aes_128_gcm", "aes-256-gcm", "aead_aes_256_gcm", "chacha20-poly1305", "aead_chacha20_poly1305", "chacha20-ietf-poly1305", "xchacha20-poly1305", "aead_xchacha20_poly1305", "xchacha20-ietf-poly1305", "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
					default:
						return fail("Shadowsocks cipher method")
					}
				}
				client := model.Client{Email: stringField(c, "email"), Password: password, Method: method}
				if err := ValidateShadowsocksKeys(&in, []model.Client{client}, nil); err != nil {
					// Different 2022 ciphers require different key lengths. A new
					// secondary membership mints its own key when another selected
					// 2022 cipher legitimately uses the supplied key.
					canMint := false
					if in.Id != anchorID && shadowsocksMethodKeySize(effectiveMethod) > 0 {
						for _, other := range inbounds {
							if other.Id == in.Id || other.Protocol != model.Shadowsocks {
								continue
							}
							otherMethod := inboundMethod(other.Settings)
							if other.Id == anchorID && stringField(c, "method") != "" {
								otherMethod = stringField(c, "method")
							}
							otherSize := shadowsocksMethodKeySize(otherMethod)
							if otherSize > 0 && otherSize != shadowsocksMethodKeySize(effectiveMethod) && validShadowsocksKey(password, otherSize) {
								canMint = true
								break
							}
						}
					}
					if !canMint {
						return fmt.Errorf("inbound %q (id %d, %s): %w", in.Remark, in.Id, in.Protocol, err)
					}
				}
			}
			if in.Protocol == model.VLESS {
				if flow := stringField(c, "flow"); flow != "" && flow != "xtls-rprx-vision" {
					return fail("flow")
				}
				if stringField(c, "encryption") != "" {
					return fail("inbound encryption")
				}
			}
			if in.Protocol == model.VMESS {
				switch strings.ToLower(stringField(c, "security")) {
				case "", "auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero":
				default:
					return fail("security")
				}
			}
			if in.Protocol == model.WGC || in.Protocol == model.AWG || in.Protocol == model.WireGuard {
				checkKeys := func(m map[string]any, requirePrivate bool) error {
					private := stringField(m, "privKey")
					public := stringField(m, "pubKey")
					psk := stringField(m, "psk")
					if requirePrivate && private == "" {
						return fail("device private key")
					}
					if public != "" && private == "" {
						return fail("device private key")
					}
					if private != "" {
						key, err := wgtypes.ParseKey(private)
						if err != nil {
							return fail("private key")
						}
						if public != "" && public != key.PublicKey().String() {
							return fail("public key")
						}
					}
					if psk != "" {
						if _, err := wgtypes.ParseKey(psk); err != nil {
							return fail("preshared key")
						}
					}
					return nil
				}
				if err := checkKeys(c, false); err != nil {
					return err
				}
				if raw, ok := c["devices"]; ok && raw != nil {
					devices, ok := raw.([]any)
					if !ok {
						return fail("devices")
					}
					for _, v := range devices {
						device, ok := v.(map[string]any)
						if !ok {
							return fail("device")
						}
						if err := checkKeys(device, true); err != nil {
							return err
						}
					}
				}
				if raw, ok := c["addresses"]; ok && raw != nil {
					addresses, ok := raw.([]any)
					if !ok {
						return fail("addresses")
					}
					for _, v := range addresses {
						value, ok := v.(string)
						if !ok {
							return fail("address")
						}
						if _, _, err := net.ParseCIDR(value); err != nil {
							return fail("address")
						}
					}
				}
			}
			if in.Protocol == model.GRE {
				if raw, ok := c["peers"]; ok && raw != nil {
					peers, ok := raw.([]any)
					if !ok {
						return fail("peers")
					}
					for _, v := range peers {
						peer, ok := v.(map[string]any)
						if !ok {
							return fail("peer")
						}
						ip := stringField(peer, "peerIp")
						if ip != "" && net.ParseIP(ip).To4() == nil {
							return fail("peer IP")
						}
					}
				}
			}
			switch in.Protocol {
			case model.L2TP, model.PPTP, model.OPENVPN, model.OPENCONNECT, model.SSTP, model.IKEV2, model.SSH:
				name := stringField(c, "vpnUsername")
				if anchor == in.Protocol {
					name = stringField(c, "id")
				}
				if !validText(name) {
					return fail("VPN username")
				}
				if err := ValidateVpnUsername(name); err != nil {
					return fmt.Errorf("inbound %q (id %d, %s): %w", in.Remark, in.Id, in.Protocol, err)
				}
			case model.Hysteria, model.Hysteria2:
				if !validText(stringField(c, "auth")) {
					return fail("auth")
				}
			case model.MTPROTO:
				raw, err := hex.DecodeString(stringField(c, "secret"))
				if err != nil || len(raw) != 16 {
					return fail("MTProto secret (32 hexadecimal characters)")
				}
			}
		}
	}
	return nil
}
