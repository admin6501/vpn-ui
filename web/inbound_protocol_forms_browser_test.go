package web

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/controller"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/op/go-logging"
)

func TestInboundProtocolFormsBrowserRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable Chromium checks")
	}
	gin.SetMode(gin.TestMode)
	logger.InitLogger(logging.CRITICAL)
	for _, legacy := range []bool{false, true} {
		for _, lang := range []string{"en-US", "fa-IR"} {
			t.Run(lang+"/legacy="+strconv.FormatBool(legacy), func(t *testing.T) {
				t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
				if err := database.InitDB(config.GetDBPath()); err != nil {
					t.Fatal(err)
				}
				db := database.GetDB()
				var root model.User
				if err := db.Where("is_super_admin = ?", true).First(&root).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&model.Setting{Key: "legacyInboundForm", Value: strconv.FormatBool(legacy)}).Error; err != nil {
					t.Fatal(err)
				}
				(&service.AccountService{}).MigrationAccounts()
				if err := (&service.GroupService{}).Migrate(); err != nil {
					t.Fatal(err)
				}
				if err := locale.InitLocalizer(i18nFS, &service.SettingService{}); err != nil {
					t.Fatal(err)
				}
				tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, args ...string) string { return locale.I18n(locale.Web, key, args...) }})
				if err != nil {
					t.Fatal(err)
				}
				r := gin.New()
				r.SetHTMLTemplate(tpl)
				r.HTMLRender, err = locale.NewHTMLRender(r.HTMLRender)
				if err != nil {
					t.Fatal(err)
				}
				r.Use(locale.LocalizerMiddleware(), func(c *gin.Context) {
					copy := root
					c.Set("LOGIN_USER_ROW", &copy)
					c.Set("base_path", "/test/")
					c.Next()
				})
				controller.NewXUIController(r.Group("/test"))
				controller.NewClientsController(r.Group("/test/panel/api/clients"))
				controller.NewInboundController(r.Group("/test/panel/api/inbounds"))
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
					if strings.HasPrefix(rq.URL.Path, "/test/assets/") {
						http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, rq)
						return
					}
					r.ServeHTTP(w, rq)
				}))
				defer srv.Close()
				script, _ := filepath.Abs("../test_unit/inbound_protocol_forms_browser_test.cjs")
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "node", script)
				cmd.Env = append(os.Environ(), "INBOUND_CREATE_URL="+srv.URL+"/test/", "NODE_UI_LANG="+lang)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("browser: %v\n%s", err, out)
				} else {
					t.Log(string(out))
				}
			})
		}
	}
}
