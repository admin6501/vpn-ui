package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestNodeAmneziaAssignmentSurvivesWireProjection(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	vless, account := seedNodeAccount(t, n, 49212, "multi-core@example.com")
	priv, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	server, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	awg := seedInboundWithClients(t, model.AWG, 51821, []map[string]any{{"email": account.Email, "enable": true, "privKey": priv.String(), "pubKey": priv.PublicKey().String(), "devices": []map[string]any{{"privKey": priv.String(), "pubKey": priv.PublicKey().String()}}, "slot": 0}})
	var settings map[string]any
	if err := json.Unmarshal([]byte(awg.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	settings["serverPrivKey"] = server.String()
	settings["serverPubKey"] = server.PublicKey().String()
	settings["ipRanges"] = []string{"10.7.0.0/24"}
	settings["userLimit"] = 1
	raw, _ := json.Marshal(settings)
	awg.Settings = string(raw)
	awg.NodeID = n.ID
	awg.StreamSettings = `{}`
	if err := database.GetDB().Save(awg).Error; err != nil {
		t.Fatal(err)
	}
	if err := (&AccountService{}).SyncInboundAccounts(database.GetDB(), awg.Id); err != nil {
		t.Fatal(err)
	}
	snap, err := svc.Snapshot(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var received nodewire.Snapshot
	if err = json.Unmarshal(wire, &received); err != nil {
		t.Fatal(err)
	}
	// A separate worker DB prevents central rows from hiding missing wire fields.
	newAccountsDB(t)
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	t.Setenv("VPNUI_NODE_AGENT", "1")
	if _, err := ApplyNodeSnapshot(&received, map[string]bool{"xray": true, "awg": true}); err != nil {
		t.Fatal(err)
	}
	var saved model.Inbound
	if err := database.GetDB().First(&saved, awg.Id).Error; err != nil {
		t.Fatal(err)
	}
	var projected awgSettings
	if err := json.Unmarshal([]byte(saved.Settings), &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected.Clients) != 1 || projected.Clients[0].deviceList()[0].PubKey != priv.PublicKey().String() {
		t.Fatal("Amnezia device credentials changed on the wire")
	}
	var sibling model.Inbound
	if err := database.GetDB().First(&sibling, vless.Id).Error; err != nil || !sibling.Enable {
		t.Fatal("Amnezia assignment disabled unrelated VLESS", err)
	}
	if _, err := (&XrayService{}).GetXrayConfig(); err != nil {
		t.Fatal("Amnezia assignment broke generated Xray configuration", err)
	}
}
