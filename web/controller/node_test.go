package controller

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNodeAgentHTTPAuthenticationAndScope(t *testing.T) {
	newIdorFixture(t)
	svc := &service.NodeService{}
	n, pair, err := svc.Create("Node", "node.example.com")
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	if _, err = svc.Enroll(nodewire.Enrollment{PairToken: pair, AgentToken: token, Architecture: "amd64"}); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	RegisterNodeAgentRoutes(r.Group("/private"))
	for _, tc := range []struct {
		id     int
		token  string
		body   string
		status int
	}{
		{n.ID, "", "{}", 403}, {n.ID, strings.Repeat("b", 64), "{}", 403}, {n.ID + 1, token, "{}", 403},
		{n.ID, token, `{"version":1,"sequence":1}`, 200},
		{n.ID, token, `{"version":1,"sequence":3}`, 409},
	} {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/private/node-agent/%d/sync", tc.id), strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("status=%d want=%d", w.Code, tc.status)
		}
	}
}
func TestNodeAdministrationRejectsRepresentative(t *testing.T) {
	f := newIdorFixture(t)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("LOGIN_USER_ROW", f.reza)
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewNodeController(r.Group("/panel"))
	req := httptest.NewRequest("POST", "/panel/nodes/add", strings.NewReader(`{"name":"Unauthorized","address":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var count int64
	database.GetDB().Model(&model.RemoteNode{}).Count(&count)
	if count != 0 || w.Code == 200 && !strings.Contains(w.Body.String(), `"success":false`) {
		t.Fatal("representative changed node infrastructure", w.Code, w.Body.String())
	}
}

func TestNodeBrowserFormPostsCreateUpdateAndInstall(t *testing.T) {
	f := newIdorFixture(t)
	f.ali.IsSuperAdmin = true
	router := gin.New()
	router.Use(withUser(f.ali))
	NewNodeController(router.Group("/panel"))
	post := func(path string, body url.Values) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("POST", path, strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var msg struct {
			Success bool `json:"success"`
		}
		if json.Unmarshal(w.Body.Bytes(), &msg) != nil || !msg.Success {
			t.Fatalf("browser form request failed: %s %s", path, w.Body.String())
		}
		return w
	}
	post("/panel/nodes/add", url.Values{"name": {"Germany"}, "address": {"de.example.com"}})
	var node model.RemoteNode
	if err := database.GetDB().Where("name = ?", "Germany").First(&node).Error; err != nil {
		t.Fatal(err)
	}
	post(fmt.Sprintf("/panel/nodes/%d/update", node.ID), url.Values{"name": {"Renamed"}, "address": {"de2.example.com"}, "enable": {"false"}})
	database.GetDB().First(&node, node.ID)
	if node.Enable || node.Name != "Renamed" || node.Address != "de2.example.com" {
		t.Fatal("form update did not bind")
	}
	post(fmt.Sprintf("/panel/nodes/%d/install", node.ID), url.Values{"cores": {"xray", "openvpn"}})
	database.GetDB().First(&node, node.ID)
	var cores []string
	if err := json.Unmarshal([]byte(node.RequestedCores), &cores); err != nil || len(cores) != 2 || node.CoreGeneration != 1 {
		t.Fatal("repeated form core fields did not bind", err)
	}
}
