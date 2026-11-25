package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
)

func TestPanelActivityPrivilegedRoutesRejectDelegatedAdmins(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("LOGIN_USER_ROW", &model.User{Id: 2, IsSuperAdmin: true, Enable: true})
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
	})
	registerActivityRoutes(r.Group("/panel/api"))
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "changes"}, {http.MethodGet, "changes/settings"}, {http.MethodPost, "changes/settings"},
		{http.MethodPost, "changes/purge"}, {http.MethodPost, "logins/purge"},
		{http.MethodGet, "apps"}, {http.MethodPost, "apps"},
	} {
		req := httptest.NewRequest(endpoint.method, "/panel/api/activity/"+endpoint.path, strings.NewReader("{}"))
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		var result struct {
			Success bool   `json:"success"`
			Msg     string `json:"msg"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil ||
			response.Code != http.StatusOK || result.Success || result.Msg != "pages.admins.ownerOnly" {
			t.Errorf("%s %s: status %d, body %s", endpoint.method, endpoint.path, response.Code, response.Body.String())
		}
	}
}
