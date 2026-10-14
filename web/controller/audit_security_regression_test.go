package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/websocket"
)

func TestSecurityAuditOpenVPNAPIRejectsInjectionWithoutWriting(t *testing.T) {
	f := newIdorFixture(t)
	before := f.aliInbound.Settings
	st, _ := json.Marshal(map[string]any{"udpEnable": true, "tcpEnable": false, "tlsCryptEnable": false, "caCert": "inline-ca", "serverCert": "inline-cert", "dns1": "8.8.8.8\"\nsetenv AUDIT injected\n#"})
	body := url.Values{"protocol": {"openvpn"}, "enable": {"false"}, "settings": {string(st)}}.Encode()
	response := f.as(t, f.ali, "POST", fmt.Sprintf("/panel/api/inbounds/update/%d", f.aliInbound.Id), body)
	if !strings.Contains(response.Body.String(), `"success":false`) || !strings.Contains(response.Body.String(), "DNS") {
		t.Fatal("API did not reject bad DNS:", response.Body.String())
	}
	var saved model.Inbound
	database.GetDB().First(&saved, f.aliInbound.Id)
	if saved.Settings != before || saved.Protocol != model.VMESS {
		t.Fatal("invalid config changed the stored inbound")
	}
}

func TestSecurityAuditCertificatePathCreateUpdateImport(t *testing.T) {
	f := newIdorFixture(t)
	stream := `{"network":"tcp","security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/etc/forbidden-secret","keyFile":"/etc/forbidden-key"}]}}`
	for _, tc := range []struct{ path, body string }{
		{"/panel/api/inbounds/add", url.Values{"protocol": {"vless"}, "streamSettings": {stream}, "port": {"41999"}, "enable": {"false"}, "settings": {`{"clients":[]}`}}.Encode()},
		{fmt.Sprintf("/panel/api/inbounds/update/%d", f.aliInbound.Id), url.Values{"enable": {"false"}, "streamSettings": {stream}}.Encode()},
		{"/panel/api/inbounds/import", url.Values{"data": {`{"protocol":"vless","enable":false,"port":41999,"settings":"{\"clients\":[]}","streamSettings":` + strconvJSONString(stream) + `}`}}.Encode()},
	} {
		response := f.as(t, f.ali, "POST", tc.path, tc.body)
		if !strings.Contains(response.Body.String(), `"success":false`) || !strings.Contains(response.Body.String(), "external certificate file path") {
			t.Fatalf("path authoring bypassed through %s: %s", tc.path, response.Body.String())
		}
	}
	var saved model.Inbound
	database.GetDB().First(&saved, f.aliInbound.Id)
	if saved.StreamSettings != f.aliInbound.StreamSettings {
		t.Fatal("refused certificate changed stored binding")
	}
}
func strconvJSONString(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestSecurityAuditSSHHostKeyRedactedWithClientCredentialsPreserved(t *testing.T) {
	f := newResellerFixture(t)
	raw := `{"hostKey":"AUDIT_PRIVATE_HOST_KEY","clients":[{"id":"sara-ssh","email":"sara-client","password":"sara-password","enable":false},{"id":"ali-ssh","email":"ali-client","password":"ali-password","enable":true}]}`
	if err := database.GetDB().Model(f.aliInbound).Updates(map[string]any{"protocol": "ssh", "settings": raw}).Error; err != nil {
		t.Fatal(err)
	}
	response := f.as(t, f.sara, "GET", "/panel/api/clients/inbounds", "")
	for _, secret := range []string{"AUDIT_PRIVATE_HOST_KEY", "ali-password"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("representative response exposed %s", secret)
		}
	}
	for _, credential := range []string{"sara-ssh", "sara-password"} {
		if !strings.Contains(response.Body.String(), credential) {
			t.Fatalf("own SSH credential missing %s: %s", credential, response.Body.String())
		}
	}
	// WebSocket uses the same scoped read model as REST; no redaction may write
	// back and rotate the server key or invalidate existing host-key pins.
	rows, err := (&service.InboundService{}).GetInboundsFor(f.sara)
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(rows)
	if strings.Contains(string(serialized), "AUDIT_PRIVATE_HOST_KEY") {
		t.Fatal("scoped socket read model leaked SSH host key")
	}
	var saved model.Inbound
	database.GetDB().First(&saved, f.aliInbound.Id)
	if !strings.Contains(saved.Settings, "AUDIT_PRIVATE_HOST_KEY") {
		t.Fatal("redaction changed persisted SSH identity")
	}
}

func TestSecurityAuditOverviewRESTPermissionsAndScope(t *testing.T) {
	f := newResellerFixture(t)
	status := &service.Status{}
	status.NetTraffic.Sent = 123456789
	status.NetIO.Up = 912345678
	for _, path := range []string{"status", "serverName", "panelLocation", "cpuHistory/minute", "getXrayVersion", "distroStatus", "geofileStatus", "geofileProgress"} {
		r := gin.New()
		r.Use(withUser(f.sara))
		(&ServerController{lastStatus: status}).initRouter(r.Group("/panel/api/server"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/panel/api/server/"+path, nil))
		if !strings.Contains(w.Body.String(), "pages.admins.forbidden") {
			t.Fatalf("missing overview guard on %s: %s", path, w.Body.String())
		}
	}
	if err := database.GetDB().Model(&model.ResellerProfile{}).Where("user_id=?", f.sara.Id).Update("allow_overview", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, user := range []*model.User{f.sara, f.ali} {
		r := gin.New()
		r.Use(withUser(user))
		(&ServerController{lastStatus: status}).initRouter(r.Group("/panel/api/server"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/panel/api/server/status", nil))
		if !strings.Contains(w.Body.String(), `"success":true`) {
			t.Fatal("authorized overview denied:", w.Body.String())
		}
		exposed := strings.Contains(w.Body.String(), "123456789")
		if exposed == user.IsRepresentativeAccount() {
			t.Fatal("global traffic scope is wrong:", w.Body.String())
		}
	}
	if status.NetTraffic.Sent != 123456789 {
		t.Fatal("projection mutated shared status")
	}
	// UUID utilities remain usable from the Clients page without Overview.
	database.GetDB().Model(&model.ResellerProfile{}).Where("user_id=?", f.sara.Id).Update("allow_overview", false)
	r := gin.New()
	r.Use(withUser(f.sara))
	(&ServerController{}).initRouter(r.Group("/panel/api/server"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/panel/api/server/getNewUUID", nil))
	if !strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatal("client creation utility was blocked by Overview")
	}
}

func TestSecurityAuditWebsocketOverviewRevocationIsLive(t *testing.T) {
	newPermTestDB(t)
	db := database.GetDB()
	user := model.User{Enable: true, Username: "live-overview-user", Password: "binding", Permissions: model.PermAccessOverview | model.PermAccessInbounds}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	messages := make(chan []byte, 4)
	r := gin.New()
	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		(&WebSocketController{}).writePump(&websocket.Client{ID: "overview-security", Send: messages}, conn, &user)
	})
	server := httptest.NewServer(r)
	defer server.Close()
	conn, _, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer close(messages)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	status := []byte(`{"type":"status","payload":{"netTraffic":{"sent":123456789}},"time":1}`)
	messages <- status
	if _, msg, err := conn.ReadMessage(); err != nil || string(msg) != string(status) {
		t.Fatal("authorized status not delivered", err)
	}
	if err := db.Model(&model.User{}).Where("id=?", user.Id).UpdateColumn("permissions", model.PermAccessInbounds).Error; err != nil {
		t.Fatal(err)
	}
	messages <- status
	messages <- []byte(`{"type":"invalidate","payload":{"inbounds":true},"time":2}`)
	_, msg, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(msg), `"invalidate"`) {
		t.Fatalf("revoked overview leaked or legitimate client event lost: %s %v", msg, err)
	}
}

func TestSecurityAuditWebsocketRepresentativeStatusScope(t *testing.T) {
	f := newResellerFixture(t)
	message := []byte(`{"type":"status","payload":{"netTraffic":{"sent":123456789},"netIO":{"up":912345678},"netIOByIface":[{"name":"eth0","up":12345}]},"time":1}`)
	if _, ok := filterSocketMessage(f.sara, message); ok {
		t.Fatal("representative without overview received status")
	}
	database.GetDB().Model(&model.ResellerProfile{}).Where("user_id=?", f.sara.Id).Update("allow_overview", true)
	result, ok := filterSocketMessage(f.sara, message)
	if !ok || strings.Contains(string(result), "123456789") || strings.Contains(string(result), "eth0") {
		t.Fatal("global representative traffic not redacted")
	}
}

func TestSecurityAuditTelegramBackupIsOwnerPOSTOnly(t *testing.T) {
	newPermTestDB(t)
	for _, tc := range []struct {
		name    string
		user    *model.User
		allowed bool
	}{
		{"delegated overview manager", &model.User{Enable: true, Permissions: model.PermOverviewManage | model.PermAccessOverview}, false},
		{"nonowner superadmin", &model.User{Enable: true, IsSuperAdmin: true}, false},
		{"owner", &model.User{Enable: true, IsSuperAdmin: true, IsPanelOwner: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(withUser(tc.user))
			// The bot is deliberately not running: no Telegram request or backup is sent.
			(&APIController{}).registerBackupRoute(r.Group("/panel/api"))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/panel/api/backuptotgbot", nil))
			if strings.Contains(w.Body.String(), `"success":true`) != tc.allowed {
				t.Fatal("incorrect owner backup authorization:", w.Body.String())
			}
			w = httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panel/api/backuptotgbot", nil))
			if w.Code != http.StatusNotFound {
				t.Fatal("GET still triggers a backup")
			}
		})
	}
}
