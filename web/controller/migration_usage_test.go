package controller

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
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
