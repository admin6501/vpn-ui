package controller

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestGroupsFreshInboundAndDeleteRecreatePreserveAccount(t *testing.T) {
	for _, protocol := range []model.Protocol{model.VLESS, model.VMESS, model.Shadowsocks} {
		t.Run(string(protocol), func(t *testing.T) {
			env := newBulkMembershipEnv(t)
			db := database.GetDB()
			if err := db.Create(&model.Setting{Key: "accountsMigratedAt", Value: "verified"}).Error; err != nil {
				t.Fatal(err)
			}
			groups := &service.GroupService{}
			if err := groups.Migrate(); err != nil {
				t.Fatal(err)
			}
			var groupCount int64
			db.Model(&model.ClientGroup{}).Count(&groupCount)
			if groupCount != 0 {
				t.Fatal("fixture must start without groups")
			}
			settings := `{"clients":[]}`
			if protocol == model.Shadowsocks {
				settings = `{"method":"chacha20-ietf-poly1305","password":"listener-password","network":"tcp,udp","clients":[]}`
			}
			form := url.Values{"protocol": {string(protocol)}, "port": {"46731"}, "remark": {"same-name"}, "enable": {"false"}, "settings": {settings}, "streamSettings": {`{"network":"tcp","security":"none"}`}, "sniffing": {`{}`}}
			create := func() model.Inbound {
				t.Helper()
				got := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/add", form.Encode())
				if got["success"] != true {
					t.Fatal("listener creation must not require groups", got)
				}
				var row model.Inbound
				if err := db.Where("port = ?", 46731).First(&row).Error; err != nil {
					t.Fatal(err)
				}
				var parsed struct {
					Clients []json.RawMessage `json:"clients"`
				}
				if err := json.Unmarshal([]byte(row.Settings), &parsed); err != nil || len(parsed.Clients) != 0 {
					t.Fatal("created listener has unexpected accounts", row.Settings, err)
				}
				return row
			}
			first := create()
			db.Model(&model.ClientGroup{}).Count(&groupCount)
			if groupCount != 0 {
				t.Fatal("creation silently assigned a group")
			}
			group, _, err := groups.Save(env.admin, 0, "customer-group", []int{first.Id})
			if err != nil {
				t.Fatal(err)
			}
			accountSettings := `{"clients":[{"email":"kept-customer","id":"11111111-1111-4111-8111-111111111111","password":"kept-password","subId":"kept-subscription","totalGB":10737418240,"enable":true}]}`
			created := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/saveAccount", url.Values{"groupIds": {strconv.Itoa(group.ID)}, "settings": {accountSettings}}.Encode())
			if created["success"] != true {
				t.Fatal(created)
			}
			var before model.Account
			if err := db.Where("email = ?", "kept-customer").First(&before).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", before.Email).Updates(map[string]any{"up": 1024, "down": 2048, "all_time": 4096}).Error; err != nil {
				t.Fatal(err)
			}
			deleted := bulkMembershipPost(t, env.admin, fmt.Sprintf("/panel/api/inbounds/del/%d", first.Id), "")
			if deleted["success"] != true {
				t.Fatal(deleted)
			}
			second := create()
			if first.Id == second.Id {
				t.Fatal("deleted identity reused")
			}
			var links int64
			db.Model(&model.GroupInbound{}).Where("inbound_id IN ?", []int{first.Id, second.Id}).Count(&links)
			if links != 0 {
				t.Fatal("deleted group link attached a new listener", links)
			}
			if _, _, err := groups.Save(env.admin, group.ID, group.Name, []int{second.Id}); err != nil {
				t.Fatal("cannot put replacement listener back in the group", err)
			}
			var after model.Account
			if err := db.First(&after, before.Id).Error; err != nil {
				t.Fatal("deleting listener deleted its account", err)
			}
			if before.UUID != after.UUID || before.Password != after.Password || before.SubID != after.SubID || before.TotalGB != after.TotalGB || before.Enable != after.Enable {
				t.Fatal("replacement listener changed account identity or package")
			}
			var traffic xray.ClientTraffic
			if err := db.Where("email = ?", before.Email).First(&traffic).Error; err != nil || traffic.Up != 1024 || traffic.Down != 2048 || traffic.AllTime != 4096 {
				t.Fatal("replacement lost usage", traffic, err)
			}
			ids, err := (&service.AccountService{}).InboundIdsForEmail(before.Email)
			if err != nil || len(ids) != 1 || ids[0] != second.Id {
				t.Fatal("group did not reattach the existing account", ids, err)
			}
			// The server still rejects embedded rosters; UI fixes must not bypass group authority.
			rogue := url.Values{"protocol": {string(protocol)}, "port": {"46732"}, "remark": {"forged-roster"}, "enable": {"false"}, "settings": {accountSettings}}
			denied := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/add", rogue.Encode())
			if denied["success"] != false {
				t.Fatal("embedded account bypassed group assignment", denied)
			}
		})
	}
}
