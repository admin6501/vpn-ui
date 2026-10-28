package controller

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

// Exercise the real API and prove rejected requests never write the anchor or
// account. The second inbound is deliberately the one requiring a password.
func TestGroupCreateChecksAllInboundCredentialsBeforeWriting(t *testing.T) {
	env := newBulkMembershipEnv(t)
	vless := seedMembershipInbound(t, env.admin.Id, model.VLESS, 48001, "")
	ss := seedMembershipInbound(t, env.admin.Id, model.Shadowsocks, 48002, "")
	if err := database.GetDB().Model(ss).Update("settings", `{"method":"chacha20-ietf-poly1305","clients":[]}`).Error; err != nil {
		t.Fatal(err)
	}
	gs := &service.GroupService{}
	group, _, err := gs.Save(env.admin, 0, "combined", []int{vless.Id, ss.Id})
	if err != nil {
		t.Fatal(err)
	}
	before := map[int]string{}
	for _, in := range []*model.Inbound{vless, ss} {
		var got model.Inbound
		database.GetDB().First(&got, in.Id)
		before[in.Id] = got.Settings
	}
	cases := []struct {
		name, uuid, password string
		want                 string
	}{
		{"empty password", "d0af1f64-5717-4562-b3fc-2c963f66afa6", "", "password"},
		{"whitespace password", "d0af1f64-5717-4562-b3fc-2c963f66afa6", "   ", "password"},
		{"control character", "d0af1f64-5717-4562-b3fc-2c963f66afa6", "bad\nsecret", "password"},
		{"invalid UUID", "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz", "valid-password", "client ID"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			email := fmt.Sprintf("rejected-%d", i)
			payload, _ := json.Marshal(map[string]any{"clients": []map[string]any{{"email": email, "id": tc.uuid, "password": tc.password, "enable": true}}})
			form := url.Values{"id": {strconv.Itoa(vless.Id)}, "groupIds": {strconv.Itoa(group.ID)}, "settings": {string(payload)}}
			result := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/addClient", form.Encode())
			if result["success"] != false || !strings.Contains(fmt.Sprint(result["msg"]), tc.want) {
				t.Fatalf("expected actionable refusal (%s): %v", tc.want, result)
			}
			var count int64
			database.GetDB().Model(&model.Account{}).Where("email = ?", email).Count(&count)
			if count != 0 {
				t.Fatal("rejected client was persisted")
			}
			for id, original := range before {
				var got model.Inbound
				database.GetDB().First(&got, id)
				if got.Settings != original {
					t.Fatalf("inbound %d changed on rejection", id)
				}
			}
		})
	}
	payload := `{"clients":[{"email":"accepted","id":"d0af1f64-5717-4562-b3fc-2c963f66afa6","password":"valid-password","enable":true}]}`
	form := url.Values{"id": {strconv.Itoa(vless.Id)}, "groupIds": {strconv.Itoa(group.ID)}, "settings": {payload}}
	result := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/addClient", form.Encode())
	if result["success"] != true {
		t.Fatalf("valid group client rejected: %v", result)
	}
	if !inboundHolds(t, vless.Id, "accepted") || !inboundHolds(t, ss.Id, "accepted") {
		t.Fatal("valid client was not provisioned to both inbounds")
	}
	// The accepted database entries must also parse in the real Xray binary.
	binary, _ := filepath.Abs(filepath.Join("..", "..", "corebundle", "core", runtime.GOARCH, "xray"))
	if _, err := os.Stat(binary); err != nil {
		t.Log("real Xray binary unavailable; skipping configuration check")
		return
	}
	var built []map[string]any
	for _, id := range []int{vless.Id, ss.Id} {
		var in model.Inbound
		if err := database.GetDB().First(&in, id).Error; err != nil {
			t.Fatal(err)
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(in.Settings), &settings); err != nil {
			t.Fatal(err)
		}
		if in.Protocol == model.VLESS {
			settings["decryption"] = "none"
		}
		if in.Protocol == model.Shadowsocks {
			settings["network"] = "tcp,udp"
		}
		built = append(built, map[string]any{"tag": in.Tag, "listen": "127.0.0.1", "port": in.Port, "protocol": in.Protocol, "settings": settings})
	}
	config, _ := json.Marshal(map[string]any{"log": map[string]any{"loglevel": "none"}, "inbounds": built, "outbounds": []map[string]any{{"protocol": "freedom"}}})
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, config, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "run", "-test", "-config", file)
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(filepath.Dir(binary)))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("valid group stopped real Xray: %v: %s", err, out)
	}
}

func TestRepresentativeInvalidGroupClientDoesNotChargeBalance(t *testing.T) {
	f := newResellerFixture(t)
	db := database.GetDB()
	syncMembershipAccounts(t)
	ss := seedMembershipInbound(t, f.ali.Id, model.Shadowsocks, 48931, "")
	if err := db.Model(ss).Update("settings", `{"method":"chacha20-ietf-poly1305","clients":[]}`).Error; err != nil {
		t.Fatal(err)
	}
	for _, u := range []*model.User{f.ali, f.sara} {
		if err := db.Create(&model.InboundAccess{UserId: u.Id, InboundId: ss.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	root := *f.ali
	root.IsSuperAdmin = true
	group, _, err := (&service.GroupService{}).Save(&root, 0, "mixed", []int{f.aliInbound.Id, ss.Id})
	if err != nil {
		t.Fatal(err)
	}
	before := f.spent(t)
	anchorBefore := f.aliSettings(t)
	form := url.Values{"id": {strconv.Itoa(f.aliInbound.Id)}, "groupIds": {strconv.Itoa(group.ID)}, "settings": {`{"clients":[{"email":"bot-client","id":"d0af1f64-5717-4562-b3fc-2c963f66afa6","password":"","totalGB":10737418240,"enable":true}]}`}}
	w := f.as(t, f.sara, "POST", "/panel/api/inbounds/addClient", form.Encode())
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["success"] != false || !strings.Contains(fmt.Sprint(result["msg"]), "password") {
		t.Fatalf("request not rejected for the missing password: %v", result)
	}
	if f.spent(t) != before {
		t.Fatal("rejected request charged the representative")
	}
	if f.aliSettings(t) != anchorBefore {
		t.Fatal("rejected request changed anchor inbound")
	}
	var n int64
	db.Model(&model.Account{}).Where("email = ?", "bot-client").Count(&n)
	if n != 0 {
		t.Fatal("rejected request created an account")
	}
}
