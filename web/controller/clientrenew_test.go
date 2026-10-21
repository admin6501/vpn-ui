package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestClientRenewActualRouteAndLegacyPrompt(t *testing.T) {
	env := newBulkMembershipEnv(t)
	account, err := accountService.SaveAccountWithoutInbound(map[string]any{
		"email": "legacy-renew", "enable": false, "totalGB": float64(10 << 30),
		"expiryTime": float64(time.Now().UnixMilli() - 86400000), "password": "keep-password", "subId": "keep-sub",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := "/panel/api/inbounds/renewClient/" + account.Email
	msg := bulkMembershipPost(t, env.admin, path, "")
	if msg["success"] != false {
		t.Fatal("unknown legacy duration should ask for days")
	}
	msg = bulkMembershipPost(t, env.admin, path, "renewalDuration=2592000000")
	mustSucceed(t, msg, "renewing a client without memberships")
	got, _ := accountService.GetAccountByEmail(account.Email)
	if !got.Enable || got.TotalGB != 10<<30 || got.SubID != "keep-sub" || got.Password != "keep-password" || got.RenewalDuration != 2592000000 {
		t.Fatal("wrong renewal result")
	}
}

func TestClientRenewRequiresUpdateAndResetGrantsWithNarrowerScope(t *testing.T) {
	newBulkMembershipEnv(t)
	for _, tc := range []struct {
		name, permissions string
		allowed, own      bool
	}{
		{"update-only", `{"users":{"update":{"scope":1}}}`, false, true},
		{"reset-only", `{"users":{"reset_usage":{"scope":1}}}`, false, true},
		{"update-all-reset-own", `{"users":{"update":{"scope":2},"reset_usage":{"scope":1}}}`, true, true},
		{"update-own-reset-all", `{"users":{"update":{"scope":1},"reset_usage":{"scope":2}}}`, true, true},
		{"all", `{"users":{"update":{"scope":2},"reset_usage":{"scope":2}}}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := &model.User{Id: 42, Enable: true, IsReseller: true, RepresentativeRoleID: 3,
				RepresentativeRole: &model.RepresentativeRole{Permissions: tc.permissions}}
			reached := false
			r := gin.New()
			r.POST("/renewClient/:email", withUser(user), requirePerm(model.PermEditClient), requireRenewResetPermission(), func(c *gin.Context) {
				reached = true
				if user.IsReseller != tc.own {
					t.Fatal("renewal widened the more restrictive scope")
				}
				c.Status(200)
			})
			req := httptest.NewRequest(http.MethodPost, "/renewClient/customer", nil)
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
			r.ServeHTTP(httptest.NewRecorder(), req)
			if reached != tc.allowed {
				t.Fatal("renewal permission bypass or false denial")
			}
		})
	}
}

func TestClientRenewRejectsOtherAdministratorsCustomer(t *testing.T) {
	newBulkMembershipEnv(t)
	owner := &model.User{Username: "other-owner", Password: "x", Enable: true}
	actor := &model.User{Username: "renew-actor", Password: "x", Enable: true, Permissions: model.PermEditClient}
	for _, user := range []*model.User{owner, actor} {
		if err := database.GetDB().Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	seedMembershipInbound(t, owner.Id, model.VLESS, 49100, `{"id":"3fa85f64-5717-4562-b3fc-2c963f66afa6","email":"other-renew","enable":false,"totalGB":10737418240}`)
	syncMembershipAccounts(t)
	msg := bulkMembershipPost(t, actor, "/panel/api/inbounds/renewClient/other-renew", "renewalDuration=0")
	if msg["success"] != false {
		t.Fatal("cross-admin renewal allowed")
	}
	got, _ := accountService.GetAccountByEmail("other-renew")
	if got.Enable {
		t.Fatal("unauthorized renewal enabled the customer")
	}
}
