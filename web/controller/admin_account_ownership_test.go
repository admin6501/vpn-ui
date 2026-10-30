package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func postOwnership(t *testing.T, actor *model.User, body, contentType string) map[string]any {
	t.Helper()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("LOGIN_USER_ROW", actor)
		c.Set("base_path", "/")
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewClientsController(r.Group("/panel/api/clients"))
	req := httptest.NewRequest(http.MethodPost, "/panel/api/clients/transferOwner", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	return result
}

func TestSuperAdminClientCreationAndTransferPreserveOwnershipAndUsage(t *testing.T) {
	env := newBulkMembershipEnv(t)
	db := database.GetDB()
	second := &model.User{Username: "second-super", Password: "x", Enable: true, IsSuperAdmin: true}
	if err := db.Create(second).Error; err != nil {
		t.Fatal(err)
	}
	inbound := seedMembershipInbound(t, env.admin.Id, model.VLESS, 49812, "")
	group, _, err := (&service.GroupService{}).Save(env.admin, 0, "ownership-test", []int{inbound.Id})
	if err != nil {
		t.Fatal(err)
	}
	settings := `{"clients":[{"email":"owner-test","id":"d0af1f64-5717-4562-b3fc-2c963f66afa6","subId":"keep-sub","enable":false,"totalGB":10737418240}]}`
	form := url.Values{"id": {strconv.Itoa(inbound.Id)}, "groupIds": {strconv.Itoa(group.ID)}, "settings": {settings}}
	mustSucceed(t, bulkMembershipPost(t, second, "/panel/api/inbounds/addClient", form.Encode()), "create by second super")
	svc := &service.AccountService{}
	account, err := svc.GetAccountByEmail("owner-test")
	if err != nil || account == nil {
		t.Fatalf("account: %+v %v", account, err)
	}
	rows, err := svc.ListAccounts(env.admin, 1, 50, "owner-test", "")
	if err != nil || len(rows.Rows) != 1 || rows.Rows[0].OwnerID != second.Id {
		t.Fatalf("creator missing: %+v %v", rows, err)
	}
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", account.Email).Updates(map[string]any{"up": 123, "down": 456, "all_time": 1000}).Error; err != nil {
		t.Fatal(err)
	}
	// A later mirror/edit must not erase the stored owner.
	if err := svc.SyncInboundAccounts(db, inbound.Id); err != nil {
		t.Fatal(err)
	}
	account, err = svc.GetAccountByEmail(account.Email)
	if err != nil || account.OwnerAdminID != second.Id {
		t.Fatalf("owner lost during sync: %+v %v", account, err)
	}
	res := postOwnership(t, env.admin, fmt.Sprintf(`{"accountId":%d,"ownerId":%d}`, account.Id, env.admin.Id), "application/json")
	mustSucceed(t, res, "transfer JSON")
	rows, err = svc.ListAccounts(env.admin, 1, 50, "owner-test", "")
	if err != nil || len(rows.Rows) != 1 || rows.Rows[0].OwnerID != env.admin.Id {
		t.Fatalf("transfer: %+v %v", rows, err)
	}
	res = postOwnership(t, env.admin, url.Values{"accountId": {strconv.Itoa(account.Id)}, "ownerId": {strconv.Itoa(second.Id)}}.Encode(), "application/x-www-form-urlencoded")
	mustSucceed(t, res, "transfer form")
	account, err = svc.GetAccountByEmail(account.Email)
	if err != nil || account.OwnerAdminID != second.Id || account.Enable || account.SubID != "keep-sub" || account.UUID != "d0af1f64-5717-4562-b3fc-2c963f66afa6" {
		t.Fatalf("changed account: %+v %v", account, err)
	}
	var traffic xray.ClientTraffic
	if err := db.Where("email = ?", account.Email).First(&traffic).Error; err != nil || traffic.Up != 123 || traffic.Down != 456 || traffic.AllTime != 1000 || traffic.Enable {
		t.Fatalf("changed traffic: %+v %v", traffic, err)
	}
	var memberships int64
	if err := db.Model(&model.AccountInbound{}).Where("account_id = ? AND inbound_id = ?", account.Id, inbound.Id).Count(&memberships).Error; err != nil || memberships != 1 {
		t.Fatalf("membership lost: %d %v", memberships, err)
	}
}

func TestSuperAdminUnservedClientKeepsCreatorAfterEdit(t *testing.T) {
	env := newBulkMembershipEnv(t)
	db := database.GetDB()
	second := &model.User{Username: "creator-super", Password: "x", Enable: true, IsSuperAdmin: true}
	if err := db.Create(second).Error; err != nil {
		t.Fatal(err)
	}
	data := url.Values{"settings": {`{"clients":[{"email":"unserved-owner","enable":true,"subId":"sub-key","totalGB":0}]}`}, "inboundIds": {""}}
	mustSucceed(t, bulkMembershipPost(t, second, "/panel/api/inbounds/saveAccount", data.Encode()), "create unserved")
	svc := &service.AccountService{}
	a, err := svc.GetAccountByEmail("unserved-owner")
	if err != nil || a.OwnerAdminID != second.Id {
		t.Fatalf("creator: %+v %v", a, err)
	}
	data = url.Values{"settings": {`{"clients":[{"email":"unserved-owner","enable":true,"subId":"sub-key","comment":"changed"}]}`}, "inboundIds": {""}}
	mustSucceed(t, bulkMembershipPost(t, env.admin, "/panel/api/inbounds/saveAccount", data.Encode()), "edit unserved")
	a, err = svc.GetAccountByEmail("unserved-owner")
	if err != nil || a.OwnerAdminID != second.Id || a.Comment != "changed" {
		t.Fatalf("owner after edit: %+v %v", a, err)
	}
}

func TestSuperAdminBatchCreationRecordsEachOwner(t *testing.T) {
	env := newBulkMembershipEnv(t)
	db := database.GetDB()
	second := &model.User{Username: "batch-super", Password: "x", Enable: true, IsSuperAdmin: true}
	if err := db.Create(second).Error; err != nil {
		t.Fatal(err)
	}
	inbound := seedMembershipInbound(t, env.admin.Id, model.VLESS, 49813, "")
	group, _, err := (&service.GroupService{}).Save(env.admin, 0, "batch-ownership", []int{inbound.Id})
	if err != nil {
		t.Fatal(err)
	}
	settings := `{"clients":[{"email":"batch-owner-a","id":"d0af1f64-5717-4562-b3fc-2c963f66afa6","enable":true},{"email":"batch-owner-b","id":"e0af1f64-5717-4562-b3fc-2c963f66afa6","enable":true}]}`
	data := url.Values{"id": {strconv.Itoa(inbound.Id)}, "groupIds": {strconv.Itoa(group.ID)}, "settings": {settings}}
	mustSucceed(t, bulkMembershipPost(t, second, "/panel/api/inbounds/addClient", data.Encode()), "batch create")
	for _, email := range []string{"batch-owner-a", "batch-owner-b"} {
		a, err := (&service.AccountService{}).GetAccountByEmail(email)
		if err != nil || a == nil || a.OwnerAdminID != second.Id {
			t.Fatalf("%s: %+v %v", email, a, err)
		}
	}
}
