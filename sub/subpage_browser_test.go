package sub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestSubscriptionPageBrowserRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable Chromium checks")
	}
	script, _ := filepath.Abs("../test_unit/subscription_page_browser_test.cjs")
	translations, _ := filepath.Abs("../web")
	for _, tc := range []struct {
		name, path, lang                           string
		total                                      int64
		expired, disabled, memberDisabled, blocked bool
	}{
		{name: "active", path: "/sub/", lang: "en-US", total: 10 * gb},
		{name: "active-persian", path: "/sub/", lang: "fa-IR", total: 10 * gb},
		{name: "representative-exhausted", path: "/sub/", lang: "en-US", total: 10 * gb, blocked: true},
		{name: "representative-exhausted-persian", path: "/sub/", lang: "fa-IR", total: 10 * gb, blocked: true},
		{name: "expired-unlimited", path: "/sub/", lang: "en-US", expired: true},
		{name: "quota-exhausted-exactly", path: "/sub/", lang: "en-US", total: 3 * gb},
		{name: "disabled-memberships", path: "/sub/", lang: "en-US", total: 10 * gb, disabled: true, memberDisabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newSubTestDB(t)
			if err := locale.InitLocalizer(os.DirFS(translations), &service.SettingService{}); err != nil {
				t.Fatal(err)
			}
			db := database.GetDB()
			if err := db.Create(&model.Setting{Key: "subPath", Value: tc.path}).Error; err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"subJsonEnable", "subClashEnable"} {
				if err := db.Create(&model.Setting{Key: key, Value: "true"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			const sid = "browser-subscription"
			expiry := time.Now().Add(24 * time.Hour).UnixMilli()
			if tc.expired {
				expiry = time.Now().Add(-24 * time.Hour).UnixMilli()
			}
			var ids []int
			for i, proto := range []model.Protocol{model.VMESS, model.Shadowsocks, model.AWG} {
				client := subClient("browser-user", sid)
				client["enable"] = !tc.memberDisabled
				settings := map[string]any{"clients": []any{client}}
				if proto == model.Shadowsocks {
					settings["method"] = "chacha20-ietf-poly1305"
					settings["password"] = "synthetic-server-password"
				}
				if proto == model.AWG {
					// Synthetic renderer-only keys: this never configures a kernel interface.
					settings["serverPubKey"] = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
					client["privKey"] = "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="
					client["pubKey"] = "AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="
					client["slot"] = 0
				}
				inbound := seedSubInbound(t, proto, 41901+i, "Browser server", client)
				raw, err := json.Marshal(settings)
				if err != nil {
					t.Fatal(err)
				}
				if err := db.Model(inbound).Updates(map[string]any{"settings": string(raw), "listen": "203.0.113.9"}).Error; err != nil {
					t.Fatal(err)
				}
				ids = append(ids, inbound.Id)
			}
			account := seedAccount(t, "browser-user", tc.total, expiry, ids...)
			if err := db.Model(account).Updates(map[string]any{"sub_id": sid, "enable": !tc.disabled, "uuid": "11111111-2222-3333-4444-555555555555", "password": "pw-browser-user", "security": "auto"}).Error; err != nil {
				t.Fatal(err)
			}
			markAccountsMigrated(t)
			seedTraffic(t, ids[0], "browser-user", gb, 2*gb, tc.total, expiry)
			if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", "browser-user").Updates(map[string]any{"enable": !tc.disabled, "last_online": time.Now().Add(-time.Minute).UnixMilli(), "all_time": 3 * gb}).Error; err != nil {
				t.Fatal(err)
			}
			var profile model.ResellerProfile
			if tc.blocked {
				role := model.RepresentativeRole{Name: "Consumption representative", DisconnectUsersWhenLimited: true}
				if err := db.Create(&role).Error; err != nil {
					t.Fatal(err)
				}
				rep := model.User{Username: "subscription-representative", Enable: true, IsReseller: true, RepresentativeRoleID: role.ID}
				if err := db.Create(&rep).Error; err != nil {
					t.Fatal(err)
				}
				profile = model.ResellerProfile{UserId: rep.Id, ConsumptionBased: true, AllowanceBytes: 3 * gb}
				if err := db.Create(&profile).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&model.ResellerClient{UserId: rep.Id, Email: account.Email, InboundId: ids[0]}).Error; err != nil {
					t.Fatal(err)
				}
				if err, _, _, _, _ := (&service.InboundService{}).AddTraffic(nil, nil); err != nil {
					t.Fatal(err)
				}
				if err := db.First(account, account.Id).Error; err != nil {
					t.Fatal(err)
				}
				if !account.Enable || !account.RepresentativeBlocked {
					t.Fatal("representative quota must independently block an enabled customer")
				}
			}
			router, err := NewServer().initRouter()
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/favicon.ico" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				router.ServeHTTP(w, r)
			}))
			defer server.Close()
			runBrowser := func(active bool) {
				t.Helper()
				expected := map[string]any{"lang": tc.lang, "active": active, "links": !tc.memberDisabled, "total": tc.total}
				raw, _ := json.Marshal(expected)
				ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "node", script)
				cmd.Env = append(os.Environ(), "SUBSCRIPTION_BROWSER_URL="+server.URL+tc.path+sid, "SUBSCRIPTION_EXPECT="+string(raw))
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("browser: %v\n%s", err, out)
				} else {
					t.Log(string(out))
				}
			}
			runBrowser(!tc.expired && !tc.disabled && !tc.blocked && (tc.total == 0 || tc.total > 3*gb))
			if tc.blocked {
				// Browser views and config downloads must never reactivate the account,
				// reset counters, or spend/replenish the representative's balance.
				if err := db.First(account, account.Id).Error; err != nil {
					t.Fatal(err)
				}
				var traffic xray.ClientTraffic
				if err := db.Where("email = ?", account.Email).First(&traffic).Error; err != nil {
					t.Fatal(err)
				}
				if !account.Enable || !account.RepresentativeBlocked || traffic.Enable || traffic.Up != gb || traffic.Down != 2*gb || traffic.AllTime != 3*gb {
					t.Fatal("viewing a blocked subscription changed enforcement or usage")
				}
				for _, id := range ids {
					var inbound model.Inbound
					if err := db.First(&inbound, id).Error; err != nil {
						t.Fatal(err)
					}
					clients, err := (&service.InboundService{}).GetClients(&inbound)
					if err != nil || len(clients) != 1 || clients[0].Enable {
						t.Fatal("viewing subscription reactivated a blocked data-plane membership", err)
					}
				}
				for _, path := range []string{tc.path + sid, "/json/" + sid, "/clash/" + sid} {
					recorder := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, path, nil)
					req.Header.Set("Accept", "text/plain")
					router.ServeHTTP(recorder, req)
					if path == tc.path+sid {
						if recorder.Code != 200 || strings.TrimSpace(recorder.Body.String()) != "" {
							t.Fatal("blocked memberships leaked into raw export")
						}
						want := header(xray.ClientTraffic{Up: gb, Down: 2 * gb, Total: tc.total, ExpiryTime: expiry})
						if recorder.Header().Get("Subscription-Userinfo") != want {
							t.Fatal("blocked raw subscription lost its real accounting header")
						}
					} else if recorder.Code != 400 {
						t.Fatalf("blocked config export %s returned %d; want 400", path, recorder.Code)
					}
				}
				if err := db.First(&profile, profile.Id).Error; err != nil {
					t.Fatal(err)
				}
				if profile.AllowanceBytes != 3*gb {
					t.Fatal("viewing the subscription changed representative allowance")
				}
				if err := db.Model(&profile).Update("allowance_bytes", 100*gb).Error; err != nil {
					t.Fatal(err)
				}
				if err, _, _, _, _ := (&service.InboundService{}).AddTraffic(nil, nil); err != nil {
					t.Fatal(err)
				}
				runBrowser(true)
				links, _, restored, err := subSvc().GetSubs(sid, "vpn.example.com")
				if err != nil || len(links) != 3 || !restored.Enable || restored.Up != gb || restored.Down != 2*gb {
					t.Fatal("replenishing representative did not restore normal exports with unchanged usage", err)
				}
				// The raw export's encoded form must also recover.
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path+sid, nil))
				body := recorder.Body.String()
				if decoded, err := base64.StdEncoding.DecodeString(body); err == nil {
					body = string(decoded)
				}
				if recorder.Code != 200 || !strings.Contains(body, "vmess://") || !strings.Contains(body, "ss://") || !strings.Contains(body, "wireguard://") {
					t.Fatal("raw export did not recover after replenishment")
				}
			}
		})
	}
}
