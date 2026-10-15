package web

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
	"github.com/mhsanaei/3x-ui/v2/web/controller"
	weblocale "github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/op/go-logging"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

func TestRepresentativeSecurityBrowserRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable Chromium checks")
	}
	gin.SetMode(gin.TestMode)
	logger.InitLogger(logging.CRITICAL)
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range []string{"en_US", "fa_IR"} {
		if _, err := bundle.LoadMessageFile("translation/translate." + lang + ".toml"); err != nil {
			t.Fatal(err)
		}
	}
	for _, lang := range []string{"en-US", "fa-IR"} {
		for _, required := range []bool{false, true} {
			t.Run(lang+"/required="+map[bool]string{false: "false", true: "true"}[required], func(t *testing.T) {
				localizer := i18n.NewLocalizer(bundle, lang)
				tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, _ ...string) (string, error) {
					return localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
				}})
				if err != nil {
					t.Fatal(err)
				}
				if err := database.InitDB(filepath.Join(t.TempDir(), "security.db")); err != nil {
					t.Fatal(err)
				}
				hash, err := crypto.HashPasswordAsBcrypt("browser-password")
				if err != nil {
					t.Fatal(err)
				}
				rep := model.User{Username: "browser-representative", Password: hash, Enable: true, IsReseller: true, TwoFactorRequired: required}
				if err := database.GetDB().Create(&rep).Error; err != nil {
					t.Fatal(err)
				}
				r := gin.New()
				r.SetHTMLTemplate(tpl)
				store := cookie.NewStore([]byte("browser-representative-session"))
				store.Options(sessions.Options{Path: "/test/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
				r.Use(sessions.Sessions("vpn-ui", store))
				r.Use(func(c *gin.Context) {
					c.Set("base_path", "/test/")
					c.Set("I18n", func(_ weblocale.I18nType, key string, _ ...string) string {
						value, _ := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
						return value
					})
					c.Next()
				})
				controller.NewIndexController(r.Group("/test"))
				controller.NewXUIController(r.Group("/test"))
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
					if strings.HasPrefix(rq.URL.Path, "/test/assets/") {
						http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, rq)
						return
					}
					// Do not start protocol services: browser covers real auth, templates and DB writes.
					r.ServeHTTP(w, rq)
				}))
				defer srv.Close()
				script, _ := filepath.Abs("../test_unit/representative_security_browser_test.cjs")
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "node", script)
				cmd.Env = append(os.Environ(), "SECURITY_UI_BASE="+srv.URL+"/test/", "SECURITY_UI_REQUIRED="+map[bool]string{false: "false", true: "true"}[required])
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("browser: %v\n%s", err, out)
				} else {
					t.Log(string(out))
				}
				var stored model.User
				database.GetDB().First(&stored, rep.Id)
				if stored.TwoFactorEnable != required {
					t.Fatal("browser changes did not persist")
				}
			})
		}
	}
}
