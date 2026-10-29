package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
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
