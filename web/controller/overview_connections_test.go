package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

func TestRepresentativeOverviewConnectionsAreReadOnlyHostStatistics(t *testing.T) {
	for _, roleBacked := range []bool{false, true} {
		for _, manage := range []bool{false, true} {
			name := "legacy"
			if roleBacked {
				name = "imported-role"
			}
			if manage {
				name += "/manage"
			} else {
				name += "/view-only"
			}
			t.Run(name, func(t *testing.T) {
				newPermTestDB(t)
				user := newReseller(t, 81, true, manage)
				if roleBacked {
					user.IsReseller = false
					user.RepresentativeRoleID = 7
					user.RepresentativeRole = &model.RepresentativeRole{ID: 7, Permissions: `{"users":{"read":{"scope":2}}}`}
				}
				snapshot := service.Status{TcpCount: 37, UdpCount: 19}
				snapshot.NetTraffic.Sent = 123456789
				snapshot.NetTraffic.Recv = 987654321
				snapshot.NetIO.Up = 123
				snapshot.NetIO.Down = 456
				snapshot.Xray.ErrorMsg = "private-client-detail"
				original := snapshot
				controller := &ServerController{lastStatus: &snapshot}
				r := gin.New()
				r.Use(withUser(user))
				controller.initRouter(r.Group("/panel/api/server"))
				request := func() []byte {
					req := httptest.NewRequest(http.MethodGet, "/panel/api/server/status", nil)
					req.Header.Set("X-Requested-With", "XMLHttpRequest")
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
					return w.Body.Bytes()
				}
				assertStatus := func(body []byte) {
					t.Helper()
					var envelope struct {
						Obj     service.Status `json:"obj"`
						Payload service.Status `json:"payload"`
					}
					if err := json.Unmarshal(body, &envelope); err != nil {
						t.Fatal(err)
					}
					result := envelope.Obj
					if envelope.Payload.TcpCount != 0 {
						result = envelope.Payload
					}
					if result.TcpCount != 37 || result.UdpCount != 19 {
						t.Fatalf("aggregate connection counts were redacted: %s", body)
					}
					if result.NetTraffic.Sent != 0 || result.NetTraffic.Recv != 0 || result.NetIO.Up != 0 || result.NetIO.Down != 0 || result.Xray.ErrorMsg != "" {
						t.Fatalf("private traffic or errors exposed: %s", body)
					}
				}
				assertStatus(request())
				payload, err := json.Marshal(map[string]any{"type": "status", "payload": snapshot})
				if err != nil {
					t.Fatal(err)
				}
				projected, allowed := filterSocketMessage(user, payload)
				if !allowed {
					t.Fatal("authorized live status denied")
				}
				assertStatus(projected)
				if !reflect.DeepEqual(original, snapshot) {
					t.Fatal("shared cached status was mutated")
				}
				if !manage {
					response := overviewCall(t, user, http.MethodPost, "/panel/api/server/serverName", "serverName=must-not-change")
					if !strings.Contains(response, forbiddenKey) {
						t.Fatal("view-only access enabled management", response)
					}
				}
				if err := database.GetDB().Model(&model.ResellerProfile{}).Where("user_id = ?", user.Id).Update("allow_overview", false).Error; err != nil {
					t.Fatal(err)
				}
				if body := string(request()); !strings.Contains(body, `"overviewDenied":true`) {
					t.Fatal("revoked HTTP access still shows host statistics", body)
				}
				if _, allowed := filterSocketMessage(user, payload); allowed {
					t.Fatal("revoked live access still shows host statistics")
				}
			})
		}
	}
}
