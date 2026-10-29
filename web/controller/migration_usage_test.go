package controller

import (
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

func TestMigrationImportUsageRejectsReseller(t *testing.T) {
	f := newResellerFixture(t)
	result := f.as(t, f.sara, http.MethodPost, "/panel/api/inbounds/migrationImportUsage/"+f.saraEmail, `{"upload":0,"download":0,"allTime":0}`)
	if strings.Contains(result.Body.String(), `"success":true`) {
		t.Fatalf("reseller unexpectedly allowed: %s", result.Body.String())
	}
	var traffic xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", f.saraEmail).First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Up != 1000 || traffic.Down != 1000 || traffic.AllTime != 2000 {
		t.Fatalf("reseller changed usage: %+v", traffic)
	}
}

func TestMigrationDestinationMultiInboundHTTP(t *testing.T) {
	env := newBulkMembershipEnv(t)
	vless := seedMembershipInbound(t, env.admin.Id, model.VLESS, 49001, "")
	ss := seedMembershipInbound(t, env.admin.Id, model.Shadowsocks, 49002, "")
	if err := database.GetDB().Model(ss).Update("settings", `{"method":"chacha20-ietf-poly1305","clients":[]}`).Error; err != nil {
		t.Fatal(err)
	}
	group, _, err := (&service.GroupService{}).Save(env.admin, 0, "migration-test", []int{vless.Id, ss.Id})
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"clients":[{"email":"migration-test","id":"d0af1f64-5717-4562-b3fc-2c963f66afa6","password":"valid-password","subId":"test-sub","enable":false,"totalGB":1000,"expiryTime":1893456000000,"renewalDuration":86400000}]}`
	form := url.Values{"id": {strconv.Itoa(vless.Id)}, "groupIds": {strconv.Itoa(group.ID)}, "settings": {payload}}
	result := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/addClient", form.Encode())
	if result["success"] != true {
		t.Fatalf("disabled destination create failed: %v", result)
	}
	var account model.Account
	if err = database.GetDB().Where("email = ?", "migration-test").First(&account).Error; err != nil {
		t.Fatal(err)
	}
	if account.Enable || account.TotalGB != 1000 || account.ExpiryTime != 1893456000000 || account.RenewalDuration != 86400000 {
		t.Fatalf("wrong destination account: %+v", account)
	}
	ids, err := (&service.AccountService{}).InboundIdsForEmail("migration-test")
	if err != nil || len(ids) != 2 {
		t.Fatalf("memberships: %v %v", ids, err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("LOGIN_USER_ROW", env.admin)
		c.Set("base_path", "/")
		c.Set("I18n", func(locale.I18nType, string, ...string) string { return "ok" })
		c.Next()
	})
	NewInboundController(r.Group("/panel/api/inbounds"))
	req := httptest.NewRequest(http.MethodPost, "/panel/api/inbounds/migrationImportUsage/migration-test", strings.NewReader(`{"upload":11,"download":22,"allTime":55}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatalf("usage import failed: %s", w.Body.String())
	}
	var ct xray.ClientTraffic
	if err = database.GetDB().Where("email = ?", "migration-test").First(&ct).Error; err != nil {
		t.Fatal(err)
	}
	if ct.Enable || ct.Up != 11 || ct.Down != 22 || ct.AllTime != 55 {
		t.Fatalf("incorrect destination usage: %+v", ct)
	}
	body := bulkMembershipBody(t, "enable", nil, []map[string]any{{"inboundId": vless.Id, "email": "migration-test"}, {"inboundId": ss.Id, "email": "migration-test"}})
	enabled := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/bulkUpdateClients", body)
	if enabled["success"] != true {
		t.Fatalf("destination activation failed: %v", enabled)
	}
	if err = database.GetDB().Where("email = ?", "migration-test").First(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.GetDB().Where("email = ?", "migration-test").First(&ct).Error; err != nil {
		t.Fatal(err)
	}
	if !account.Enable || !ct.Enable || ct.Up != 11 || ct.Down != 22 || ct.AllTime != 55 {
		t.Fatalf("activation lost status or usage: account=%+v traffic=%+v", account, ct)
	}
	form.Set("settings", `{"clients":[{"email":"first-use-test","id":"14af1f64-5717-4562-b3fc-2c963f66afa6","password":"valid-password","subId":"first-use-sub","enable":false,"totalGB":0,"expiryTime":-86400000,"renewalDuration":86400000}]}`)
	firstUse := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/addClient", form.Encode())
	if firstUse["success"] != true {
		t.Fatalf("first-use destination create failed: %v", firstUse)
	}
	var delayed model.Account
	if err = database.GetDB().Where("email = ?", "first-use-test").First(&delayed).Error; err != nil {
		t.Fatal(err)
	}
	if delayed.Enable || delayed.TotalGB != 0 || delayed.ExpiryTime != -86400000 {
		t.Fatalf("first-use expiry or unlimited quota changed: %+v", delayed)
	}
}

func TestMigrationImportUsageSuperAdminHTTP(t *testing.T) {
	newIdorFixture(t)
	db := database.GetDB()
	admin := &model.User{Username: "root-admin", Password: "x", Enable: true, IsSuperAdmin: true}
	if err := db.Create(admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Account{Email: "dest-user", Enable: false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&xray.ClientTraffic{Email: "dest-user", Enable: false}).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("LOGIN_USER_ROW", admin)
		c.Set("base_path", "/")
		c.Set("I18n", func(locale.I18nType, string, ...string) string { return "ok" })
		c.Next()
	})
	NewInboundController(r.Group("/panel/api/inbounds"))
	request := httptest.NewRequest(http.MethodPost, "/panel/api/inbounds/migrationImportUsage/dest-user", strings.NewReader(`{"upload":11,"download":22,"allTime":55}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request)
	if !strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatalf("import failed: %s", w.Body.String())
	}
	var traffic xray.ClientTraffic
	if err := db.Where("email = ?", "dest-user").First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Up != 11 || traffic.Down != 22 || traffic.AllTime != 55 {
		t.Fatalf("incorrect counters: %+v", traffic)
	}
}
