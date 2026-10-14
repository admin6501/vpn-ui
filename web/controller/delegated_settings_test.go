package controller

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

func TestDelegatedPanelSettingsManualAndRoleGrants(t *testing.T) {
	newPermTestDB(t)
	db := database.GetDB()
	for key, value := range map[string]string{
		"tgBotToken": "private-bot-token", "ldapPassword": "private-bind-password", "twoFactorToken": "retired-global-secret",
	} {
		if err := db.Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	role := model.AdminRole{ID: 17, Name: "Settings manager", Permissions: model.PermPanelSettings}
	if err := db.Create(&model.Inbound{UserId: 99, Tag: "shared-ldap-tag", Protocol: model.VLESS, Port: 12345, Settings: `{"clients":[{"password":"must-not-leak"}]}`}).Error; err != nil {
		t.Fatal(err)
	}
	users := []*model.User{
		{Id: 2, Enable: true, Permissions: model.PermPanelSettings},
		{Id: 3, Enable: true, AdminRoleID: role.ID, AdminRole: &role},
		{Id: 4, Enable: true, IsSuperAdmin: true},
		{Id: 1, Enable: true, IsSuperAdmin: true, IsPanelOwner: true},
	}
	for _, user := range users {
		r := gin.New()
		r.Use(withUser(user))
		NewSettingController(r.Group("/panel"))
		metadataRequest := httptest.NewRequest("GET", "/panel/setting/inboundTags", nil)
		metadata := httptest.NewRecorder()
		r.ServeHTTP(metadata, metadataRequest)
		if !strings.Contains(metadata.Body.String(), "shared-ldap-tag") || strings.Contains(metadata.Body.String(), "must-not-leak") || strings.Contains(metadata.Body.String(), "settings") {
			t.Fatal("settings-only grant cannot read safe inbound metadata", metadata.Body.String())
		}
		call := func(path, body, contentType string) string {
			req := httptest.NewRequest("POST", "/panel/setting/"+path, strings.NewReader(body))
			req.Header.Set("Content-Type", contentType)
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			return w.Body.String()
		}
		read := call("all", "", "application/x-www-form-urlencoded")
		var response struct {
			Success bool
			Obj     map[string]any
		}
		if err := json.Unmarshal([]byte(read), &response); err != nil || !response.Success {
			t.Fatal(read, err)
		}
		for _, key := range []string{"tgBotToken", "ldapPassword", "webKeyFile", "subKeyFile", "externalTrafficInformURI"} {
			if _, ok := response.Obj[key]; !ok {
				t.Fatal("full settings field missing", key, read)
			}
		}
		if strings.Contains(read, "retired-global-secret") {
			t.Fatal("retired global 2FA leaked")
		}
		response.Obj["pageSize"] = 45
		response.Obj["subTitle"] = ""
		response.Obj["tgBotToken"] = "delegated-token"
		response.Obj["ldapPassword"] = "delegated-password"
		payload, _ := json.Marshal(response.Obj)
		if result := call("update", string(payload), "application/json"); !strings.Contains(result, `"success":true`) {
			t.Fatal(result)
		}
		var svc service.SettingService
		settings, err := svc.GetAllSetting()
		if err != nil || settings.PageSize != 45 || settings.SubTitle != "" || settings.TgBotToken != "delegated-token" || settings.LdapPassword != "delegated-password" || settings.TwoFactorToken != "retired-global-secret" {
			t.Fatal("full settings round trip failed", settings, err)
		}
		form := url.Values{"tgBotAPIServer": {"https://bot.example.test"}, "tgBotBackup": {"true"}, "ldapHost": {"ldap.example.test"}, "webDomain": {"panel.example.test"}, "subDomain": {"sub.example.test"}, "pageSize": {"30"}, "twoFactorToken": {"cannot-replace-authentication"}}.Encode()
		if result := call("update", form, "application/x-www-form-urlencoded"); !strings.Contains(result, `"success":true`) {
			t.Fatal("form update denied", result)
		}
		settings, _ = svc.GetAllSetting()
		if settings.PageSize != 30 || !settings.TgBotBackup || settings.TgBotAPIServer != "https://bot.example.test" || settings.LdapHost != "ldap.example.test" || settings.TwoFactorToken != "retired-global-secret" {
			t.Fatal("form update or authenticator isolation failed", settings)
		}
		// Invalid scalar inputs must still be rejected without a partial save.
		if result := call("update", `{"webPort":70000,"tgBotToken":"invalid-update"}`, "application/json"); !strings.Contains(result, `"success":false`) {
			t.Fatal("invalid settings accepted", result)
		}
		settings, _ = svc.GetAllSetting()
		if settings.TgBotToken != "delegated-token" {
			t.Fatal("invalid request partially saved")
		}
		// Invalid bodies reach the real SSL/systemd handlers but invoke no host mutations.
		for _, path := range []string{"service", "ssl/start", "ssl/preflight", "ssl/use-managed"} {
			result := call(path, `"invalid-body"`, "application/json")
			if !strings.Contains(result, `"success":false`) || strings.Contains(result, "ownerOnly") || strings.Contains(result, "pages.admins.forbidden") {
				t.Fatal("full settings route incorrectly owner-gated", path, result)
			}
		}
	}
	for _, user := range []*model.User{
		{Id: 5, Enable: true},
		{Id: 6, Enable: true, Permissions: model.PermPanelSettings, AdminRoleID: 18, AdminRole: &model.AdminRole{ID: 18}},
		{Id: 7, Enable: false, Permissions: model.PermPanelSettings},
	} {
		r := gin.New()
		r.Use(withUser(user))
		NewSettingController(r.Group("/panel"))
		metadataRequest := httptest.NewRequest("GET", "/panel/setting/inboundTags", nil)
		metadataRequest.Header.Set("X-Requested-With", "XMLHttpRequest")
		metadata := httptest.NewRecorder()
		r.ServeHTTP(metadata, metadataRequest)
		if !strings.Contains(metadata.Body.String(), `"success":false`) || strings.Contains(metadata.Body.String(), "shared-ldap-tag") {
			t.Fatal("inbound metadata bypassed settings permission", metadata.Body.String())
		}
		for _, path := range []string{"all", "update", "service", "ssl/start", "ssl/use-managed", "ssl/unassign", "ssl/rollback", "ssl/delete-profile", "ssl/adopt", "ssl/sync", "ssl/auto-renew", "ssl/nickname"} {
			req := httptest.NewRequest("POST", "/panel/setting/"+path, strings.NewReader(`{"pageSize":80}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if !strings.Contains(w.Body.String(), `"success":false`) || !strings.Contains(w.Body.String(), "forbidden") {
				t.Fatal(path, w.Body.String())
			}
		}
	}
}
