package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestGrePeerIPEditPersistsAndRendersWithoutChangingOtherFields(t *testing.T) {
	newAccountsDB(t)
	inbound := model.Inbound{Protocol: model.GRE, Listen: "192.0.2.1", Settings: `{"ipRanges":["10.9.0.0/24"],"ipsecEnable":true,"ipsecPsk":"fixture-only-key","userLimit":2,"futureFlag":9007199254740993,"clients":[{"email":"customer","enable":false,"slot":3,"subId":"keep-sub","totalGB":10737418240,"futureField":9007199254740993,"peers":[{"peerIp":"","remark":"branch","futurePeer":true},{"peerIp":"198.51.100.8","remark":"other"}]},{"email":"other","slot":4,"peers":[{},{}]}]}`}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	account := model.Account{Email: "customer", Enable: false, TotalGB: 10 << 30, SubID: "keep-sub"}
	if err := database.GetDB().Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	membership := model.AccountInbound{AccountId: account.Id, InboundId: inbound.Id, Extra: `{"email":"customer","peers":[{},{}]}`}
	if err := database.GetDB().Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	var before map[string]json.RawMessage
	json.Unmarshal([]byte(inbound.Settings), &before)
	s := &GreService{}
	if err := s.SetPeerIP(inbound.Id, "customer", 0, " 203.0.113.9 ", ""); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(&inbound, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	configs, err := s.RenderPeerConfigs(&inbound, "customer", "ignored.example")
	if err != nil || len(configs) != 2 {
		t.Fatal("render", err)
	}
	if configs[0].Dynamic || configs[0].PeerIp != "203.0.113.9" || !strings.Contains(configs[0].ConfigMikrotik, "src-address=203.0.113.9/32") || !strings.Contains(configs[0].ConfigMikrotik, "local-address=203.0.113.9") || strings.Contains(configs[0].ConfigMikrotik, "YOUR-PUBLIC-IP") {
		t.Fatal("router recipe did not use the saved public IP")
	}
	if configs[1].PeerIp != "198.51.100.8" {
		t.Fatal("another peer was changed")
	}
	if err := database.GetDB().First(&membership, "account_id = ? AND inbound_id = ?", account.Id, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(membership.Extra, `"peerIp":"203.0.113.9"`) {
		t.Fatal("membership fallback still contains the old router IP")
	}
	if err := database.GetDB().First(&account, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	if account.Enable || account.TotalGB != 10<<30 || account.SubID != "keep-sub" {
		t.Fatal("editing the router IP changed account fields")
	}
	// Removing just the intended IP makes every other JSON value identical,
	// including unknown fields and integers too large for float64.
	var after map[string]json.RawMessage
	json.Unmarshal([]byte(inbound.Settings), &after)
	var oldClients, newClients []map[string]json.RawMessage
	json.Unmarshal(before["clients"], &oldClients)
	json.Unmarshal(after["clients"], &newClients)
	var oldPeers, newPeers []map[string]json.RawMessage
	json.Unmarshal(oldClients[0]["peers"], &oldPeers)
	json.Unmarshal(newClients[0]["peers"], &newPeers)
	newPeers[0]["peerIp"] = oldPeers[0]["peerIp"]
	newClients[0]["peers"], _ = json.Marshal(newPeers)
	after["clients"], _ = json.Marshal(newClients)
	normalize := func(v any) string {
		b, _ := json.Marshal(v)
		var raw any
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.UseNumber()
		if err := dec.Decode(&raw); err != nil {
			t.Fatal(err)
		}
		b, _ = json.Marshal(raw)
		return string(b)
	}
	if normalize(before) != normalize(after) {
		t.Fatal("unrelated settings changed")
	}
	if err := s.SetPeerIP(inbound.Id, "customer", 0, "", "203.0.113.9"); err != nil {
		t.Fatal(err)
	}
	database.GetDB().First(&inbound, inbound.Id)
	configs, err = s.RenderPeerConfigs(&inbound, "customer", "ignored.example")
	if err != nil || !configs[0].Dynamic || !strings.Contains(configs[0].ConfigMikrotik, "src-address=YOUR-PUBLIC-IP/32") {
		t.Fatal("clearing IP must restore dynamic mode", err)
	}
}

func TestGrePeerIPEditRejectsInvalidAndStaleRequests(t *testing.T) {
	newAccountsDB(t)
	inbound := model.Inbound{Protocol: model.GRE, Settings: `{"clients":[{"email":"customer","peers":[{"peerIp":"203.0.113.9"}]}]}`}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	s := &GreService{}
	for _, ip := range []string{"YOUR-PUBLIC-IP", "1.2.3.4/32", "1.2.3.999", "::1", "0.0.0.0", "127.0.0.1", "224.0.0.1", "255.255.255.255", "1.2.3.4; echo injected"} {
		if err := s.SetPeerIP(inbound.Id, "customer", 0, ip, "203.0.113.9"); err == nil {
			t.Fatalf("accepted invalid IP %q", ip)
		}
	}
	for _, request := range []struct {
		email    string
		index    int
		expected string
	}{{"missing", 0, "203.0.113.9"}, {"customer", -1, "203.0.113.9"}, {"customer", 1, "203.0.113.9"}, {"customer", 0, ""}} {
		if err := s.SetPeerIP(inbound.Id, request.email, request.index, "198.51.100.5", request.expected); err == nil {
			t.Fatal("accepted missing/stale peer")
		}
	}
	var stored model.Inbound
	database.GetDB().First(&stored, inbound.Id)
	if stored.Settings != inbound.Settings {
		t.Fatal("rejected edits changed settings")
	}
	database.GetDB().Model(&inbound).Update("protocol", model.VLESS)
	if err := s.SetPeerIP(inbound.Id, "customer", 0, "198.51.100.5", "203.0.113.9"); err == nil {
		t.Fatal("accepted a non-GRE inbound")
	}
}
