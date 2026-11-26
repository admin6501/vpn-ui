package sub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

func TestSubscriptionRoutesApplySavedNameWithoutRestart(t *testing.T) {
	newSubTestDB(t)
	if err := locale.InitLocalizer(os.DirFS("../web"), &service.SettingService{}); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	settings := []model.Setting{
		{Key: "subPath", Value: "/sub/"},
		{Key: "subEncrypt", Value: "false"},
		{Key: "subJsonEnable", Value: "true"},
		{Key: "subClashEnable", Value: "true"},
		{Key: "remarkModel", Value: "-ie"},
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatal(err)
	}
	inbound := seedSubInbound(t, model.VLESS, 41891, "Server", subClient("alice", "name-sub"))
	stream := `{"network":"tcp","security":"none","externalProxy":[{"dest":"proxy.example.com","port":8443,"forceTls":"same","remark":"Edge"}]}`
	if err := db.Model(inbound).Update("stream_settings", stream).Error; err != nil {
		t.Fatal(err)
	}
	account := seedAccount(t, "alice", 0, 0, inbound.Id)
	if err := db.Model(account).Update("sub_id", "name-sub").Error; err != nil {
		t.Fatal(err)
	}
	router, err := NewServer().initRouter()
	if err != nil {
		t.Fatal(err)
	}
	fetch := func(path, accept string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", accept)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, rec.Code)
		}
		return rec.Body.String()
	}
	check := func(want string) {
		t.Helper()
		raw := fetch("/sub/name-sub/raw", "text/plain")
		names := fragments(t, []string{strings.TrimSpace(raw)})
		if len(names) != 1 || names[0] != want {
			t.Errorf("raw names = %q, want %q", names, want)
		}
		if html := fetch("/sub/name-sub", "text/html"); !strings.Contains(html, "#"+want) {
			t.Errorf("browser lost the configured link name %q", want)
		}
		var xray struct {
			Remarks string
		}
		if err := json.Unmarshal([]byte(fetch("/json/name-sub", "application/json")), &xray); err != nil || xray.Remarks != want {
			t.Errorf("JSON name = %q, error = %v, want %q", xray.Remarks, err, want)
		}
		var clash ClashConfig
		if err := yaml.Unmarshal([]byte(fetch("/clash/name-sub", "text/yaml")), &clash); err != nil ||
			len(clash.Proxies) != 1 || clash.Proxies[0]["name"] != want {
			t.Errorf("Clash proxies = %v, error = %v, want name %q", clash.Proxies, err, want)
		}
	}
	check("Server-alice")
	if err := db.Model(&model.Setting{}).Where("key = ?", "remarkModel").Update("value", "~oei").Error; err != nil {
		t.Fatal(err)
	}
	check("Edge~alice~Server")
}
