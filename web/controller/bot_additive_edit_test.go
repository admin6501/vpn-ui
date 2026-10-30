package controller

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

// The bot edits the existing client through the real panel route. This checks
// that remaining usage, identity and independent inbound switches survive.
func TestBotAdditiveEditPreservesUsageAndMemberships(t *testing.T) {
	env := newBulkMembershipEnv(t)
	vless := seedMembershipInbound(t, env.admin.Id, model.VLESS, 49201, "")
	ss := seedMembershipInbound(t, env.admin.Id, model.Shadowsocks, 49202, "")
	if err := database.GetDB().Model(ss).Update("settings", `{"method":"chacha20-ietf-poly1305","clients":[]}`).Error; err != nil {
		t.Fatal(err)
	}
	group, _, err := (&service.GroupService{}).Save(env.admin, 0, "test", []int{vless.Id, ss.Id})
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UnixMilli() + 10*86400000
	client := map[string]any{"email": "bot-renew", "id": "d0af1f64-5717-4562-b3fc-2c963f66afa6", "password": "keep-password", "subId": "keep-sub", "enable": true, "totalGB": int64(10 << 30), "expiryTime": expiry}
	settings, _ := json.Marshal(map[string]any{"clients": []any{client}})
	form := url.Values{"id": {strconv.Itoa(vless.Id)}, "groupIds": {strconv.Itoa(group.ID)}, "settings": {string(settings)}}
	mustSucceed(t, bulkMembershipPost(t, env.admin, "/panel/api/inbounds/addClient", form.Encode()), "create")
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "bot-renew").Updates(map[string]any{"up": int64(2 << 30), "down": int64(1 << 30), "all_time": int64(4 << 30)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = (&service.AccountService{}).SetMembershipEnable("bot-renew", ss.Id, false); err != nil {
		t.Fatal(err)
	}
	var inbound model.Inbound
	if err = database.GetDB().First(&inbound, vless.Id).Error; err != nil {
		t.Fatal(err)
	}
	var current struct {
		Clients []map[string]any `json:"clients"`
	}
	if err = json.Unmarshal([]byte(inbound.Settings), &current); err != nil || len(current.Clients) != 1 {
		t.Fatalf("inbound: %v %v", current, err)
	}
	entry := current.Clients[0]
	entry["totalGB"] = int64(15 << 30)
	entry["expiryTime"] = expiry + 7*86400000
	entry["enable"] = true
	settings, _ = json.Marshal(map[string]any{"clients": []any{entry}})
	form = url.Values{"id": {strconv.Itoa(vless.Id)}, "settings": {string(settings)}}
	mustSucceed(t, bulkMembershipPost(t, env.admin, "/panel/api/inbounds/updateClient/d0af1f64-5717-4562-b3fc-2c963f66afa6", form.Encode()), "extend")
	account, err := (&service.AccountService{}).GetAccountByEmail("bot-renew")
	if err != nil {
		t.Fatal(err)
	}
	if account.TotalGB != 15<<30 || account.ExpiryTime != expiry+7*86400000 || account.SubID != "keep-sub" || account.UUID != "d0af1f64-5717-4562-b3fc-2c963f66afa6" {
		t.Fatalf("account changed incorrectly: %+v", account)
	}
	var traffic xray.ClientTraffic
	if err = database.GetDB().Where("email = ?", "bot-renew").First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Up != 2<<30 || traffic.Down != 1<<30 || traffic.AllTime != 4<<30 {
		t.Fatalf("usage changed: %+v", traffic)
	}
	var membership model.AccountInbound
	if err = database.GetDB().Where("account_id = ? AND inbound_id = ?", account.Id, ss.Id).First(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if membership.Enable == nil || *membership.Enable {
		t.Fatal("disabled inbound was enabled")
	}
}

func TestBotAdditiveEditChargesRepresentativeOnlyForAddedQuota(t *testing.T) {
	f := newResellerFixture(t)
	expiry := time.Now().UnixMilli() + 10*86400000
	settings := fmt.Sprintf(`{"clients":[{"id":"ali-uuid","email":"ali-client","enable":true,"totalGB":0},{"id":"sara-uuid","email":"sara-client","enable":false,"totalGB":10737418240,"expiryTime":%d}]}`, expiry)
	if err := database.GetDB().Model(f.aliInbound).Update("settings", settings).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", f.saraEmail).Update("expiry_time", expiry).Error; err != nil {
		t.Fatal(err)
	}
	syncMembershipAccounts(t)
	client := map[string]any{"id": "sara-uuid", "email": f.saraEmail, "enable": true, "totalGB": int64(15 << 30), "expiryTime": expiry + 7*86400000}
	encoded, _ := json.Marshal(map[string]any{"clients": []any{client}})
	form := url.Values{"id": {strconv.Itoa(f.aliInbound.Id)}, "settings": {string(encoded)}}
	w := f.as(t, f.sara, "POST", "/panel/api/inbounds/updateClient/sara-uuid", form.Encode())
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response["success"] != true {
		t.Fatalf("extension failed: %s", w.Body.String())
	}
	if spent := f.spent(t); spent != 15<<30 {
		t.Fatalf("representative spent=%d", spent)
	}
	var traffic xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", f.saraEmail).First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Up != 1000 || traffic.Down != 1000 || traffic.AllTime != 2000 || traffic.Total != 15<<30 || traffic.ExpiryTime != expiry+7*86400000 {
		t.Fatalf("traffic changed incorrectly: %+v", traffic)
	}
}

func TestBotGuardedEditRejectsRepresentativeDurationMismatchWithoutCharge(t *testing.T) {
	f := newResellerFixture(t)
	expiry := time.Now().UnixMilli() + 10*86400000
	settings := fmt.Sprintf(`{"clients":[{"id":"ali-uuid","email":"ali-client","enable":true,"totalGB":0},{"id":"sara-uuid","email":"sara-client","enable":false,"totalGB":10737418240,"expiryTime":%d}]}`, expiry)
	db := database.GetDB()
	if err := db.Model(f.aliInbound).Update("settings", settings).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", f.saraEmail).Update("expiry_time", expiry).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.ResellerProfile{}).Where("user_id = ?", f.sara.Id).Update("days_per_gb", 1).Error; err != nil {
		t.Fatal(err)
	}
	syncMembershipAccounts(t)
	edit := func(days int64) map[string]any {
		desired := expiry + days*86400000
		client := map[string]any{"id": "sara-uuid", "email": f.saraEmail, "enable": true, "totalGB": int64(15 << 30), "expiryTime": desired}
		encoded, _ := json.Marshal(map[string]any{"clients": []any{client}})
		form := url.Values{"id": {strconv.Itoa(f.aliInbound.Id)}, "settings": {string(encoded)}, "expectedExpiry": {strconv.FormatInt(desired, 10)}}
		w := f.as(t, f.sara, "POST", "/panel/api/inbounds/updateClient/sara-uuid", form.Encode())
		var response map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	if got := edit(7); got["success"] != false {
		t.Fatalf("conflicting duration accepted: %v", got)
	}
	if spent := f.spent(t); spent != 10<<30 {
		t.Fatalf("rejected edit charged representative: %d", spent)
	}
	var traffic xray.ClientTraffic
	if err := db.Where("email = ?", f.saraEmail).First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Total != 10<<30 || traffic.ExpiryTime != expiry || traffic.Up != 1000 || traffic.Down != 1000 {
		t.Fatalf("rejected edit changed client: %+v", traffic)
	}
	if got := edit(5); got["success"] != true {
		t.Fatalf("matching duration rejected: %v", got)
	}
	if spent := f.spent(t); spent != 15<<30 {
		t.Fatalf("matching edit charged incorrectly: %d", spent)
	}
}

func TestBotAdditiveEditReactivatesExpiredAccount(t *testing.T) {
	env := newBulkMembershipEnv(t)
	inbound := seedMembershipInbound(t, env.admin.Id, model.VLESS, 49203, "")
	group, _, err := (&service.GroupService{}).Save(env.admin, 0, "expired-test", []int{inbound.Id})
	if err != nil {
		t.Fatal(err)
	}
	client := map[string]any{"email": "expired-bot", "id": "d0af1f64-5717-4562-b3fc-2c963f66afa6", "subId": "keep-sub", "enable": false, "totalGB": int64(10 << 30), "expiryTime": time.Now().UnixMilli() - 86400000}
	settings, _ := json.Marshal(map[string]any{"clients": []any{client}})
	form := url.Values{"id": {strconv.Itoa(inbound.Id)}, "groupIds": {strconv.Itoa(group.ID)}, "settings": {string(settings)}}
	mustSucceed(t, bulkMembershipPost(t, env.admin, "/panel/api/inbounds/addClient", form.Encode()), "create")
	if err = database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "expired-bot").Updates(map[string]any{"up": int64(5 << 30), "down": int64(5 << 30)}).Error; err != nil {
		t.Fatal(err)
	}
	var stored model.Inbound
	if err = database.GetDB().First(&stored, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var current struct {
		Clients []map[string]any `json:"clients"`
	}
	if err = json.Unmarshal([]byte(stored.Settings), &current); err != nil || len(current.Clients) != 1 {
		t.Fatal(err)
	}
	entry := current.Clients[0]
	entry["totalGB"] = int64(15 << 30)
	entry["expiryTime"] = time.Now().UnixMilli() + 7*86400000
	entry["enable"] = true
	settings, _ = json.Marshal(map[string]any{"clients": []any{entry}})
	form = url.Values{"id": {strconv.Itoa(inbound.Id)}, "settings": {string(settings)}}
	mustSucceed(t, bulkMembershipPost(t, env.admin, "/panel/api/inbounds/updateClient/d0af1f64-5717-4562-b3fc-2c963f66afa6", form.Encode()), "extend")
	account, err := (&service.AccountService{}).GetAccountByEmail("expired-bot")
	if err != nil {
		t.Fatal(err)
	}
	var traffic xray.ClientTraffic
	if err = database.GetDB().Where("email = ?", "expired-bot").First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if !account.Enable || !traffic.Enable || traffic.Up != 5<<30 || traffic.Down != 5<<30 || account.TotalGB != 15<<30 || account.ExpiryTime <= time.Now().UnixMilli() {
		t.Fatalf("expired extension incorrect: account=%+v traffic=%+v", account, traffic)
	}
}

func TestBotAdditiveEditPasswordContainingSlash(t *testing.T) {
	env := newBulkMembershipEnv(t)
	inbound := seedMembershipInbound(t, env.admin.Id, model.Trojan, 49204, "")
	group, _, err := (&service.GroupService{}).Save(env.admin, 0, "slash-test", []int{inbound.Id})
	if err != nil {
		t.Fatal(err)
	}
	client := map[string]any{"email": "slash-bot", "password": "secret/part", "subId": "keep-sub", "enable": true, "totalGB": int64(10 << 30), "expiryTime": time.Now().UnixMilli() + 86400000}
	encoded, _ := json.Marshal(map[string]any{"clients": []any{client}})
	form := url.Values{"id": {strconv.Itoa(inbound.Id)}, "groupIds": {strconv.Itoa(group.ID)}, "settings": {string(encoded)}}
	mustSucceed(t, bulkMembershipPost(t, env.admin, "/panel/api/inbounds/addClient", form.Encode()), "create")
	client["totalGB"] = int64(15 << 30)
	encoded, _ = json.Marshal(map[string]any{"clients": []any{client}})
	form = url.Values{"id": {strconv.Itoa(inbound.Id)}, "settings": {string(encoded)}}
	form.Set("expectedExpiry", strconv.FormatInt(client["expiryTime"].(int64), 10))
	mustSucceed(t, bulkMembershipPost(t, env.admin, "/panel/api/inbounds/updateClientByEmail", form.Encode()), "edit slash password")
	var account model.Account
	if err = database.GetDB().Where("email = ?", "slash-bot").First(&account).Error; err != nil {
		t.Fatal(err)
	}
	if account.TotalGB != 15<<30 || account.Password != "secret/part" || account.SubID != "keep-sub" {
		t.Fatalf("identity or quota changed: %+v", account)
	}
}

func TestBotEditByEmailRejectsOtherOwnersClient(t *testing.T) {
	f := newResellerFixture(t)
	before := f.aliSettings(t)
	client := map[string]any{"id": "ali-uuid", "email": "ali-client", "enable": true, "totalGB": int64(20 << 30)}
	encoded, _ := json.Marshal(map[string]any{"clients": []any{client}})
	form := url.Values{"id": {strconv.Itoa(f.aliInbound.Id)}, "settings": {string(encoded)}}
	w := f.as(t, f.sara, "POST", "/panel/api/inbounds/updateClientByEmail", form.Encode())
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["success"] != false || f.aliSettings(t) != before {
		t.Fatalf("foreign client edited: %s", w.Body.String())
	}
}
