package sub

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

func TestSubscriptionPageBrandUpdatesAndEscapes(t *testing.T) {
	newSubTestDB(t)
	translations, _ := filepath.Abs("../web")
	if err := locale.InitLocalizer(os.DirFS(translations), &service.SettingService{}); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	if err := db.Create(&model.Setting{Key: "subPath", Value: "/sub/"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Setting{Key: "brandName", Value: "Brand <safe>"}).Error; err != nil {
		t.Fatal(err)
	}
	inbound := seedSubInbound(t, model.VMESS, 41901, "Server", subClient("alice", "brand-sub"))
	account := seedAccount(t, "alice", 0, 0, inbound.Id)
	if err := db.Model(account).Update("sub_id", "brand-sub").Error; err != nil {
		t.Fatal(err)
	}
	markAccountsMigrated(t)
	router, err := NewServer().initRouter()
	if err != nil {
		t.Fatal(err)
	}
	check := func(brand string) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/sub/brand-sub", nil)
		req.Header.Set("Accept", "text/html")
		router.ServeHTTP(rec, req)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), brand) {
			t.Fatalf("brand %q missing, status %d", brand, rec.Code)
		}
	}
	check("Brand &lt;safe&gt;")
	if err := db.Model(&model.Setting{}).Where("key = ?", "brandName").Update("value", "Next Brand").Error; err != nil {
		t.Fatal(err)
	}
	check("Next Brand")
}
