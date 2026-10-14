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

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/controller"
	weblocale "github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

func TestDelegatedSettingsBrowserRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable Chromium checks")
	}
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range []string{"en_US", "fa_IR"} {
		if _, err := bundle.LoadMessageFile("translation/translate." + lang + ".toml"); err != nil {
			t.Fatal(err)
		}
	}
	for _, lang := range []string{"en-US", "fa-IR"} {
		for _, grant := range []string{"manual", "role", "owner"} {
			t.Run(lang+"/"+grant, func(t *testing.T) {
				localizer := i18n.NewLocalizer(bundle, lang)
				tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, _ ...string) (string, error) {
					return localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
				}})
				if err != nil {
					t.Fatal(err)
				}
				if err := database.InitDB(filepath.Join(t.TempDir(), "settings-browser.db")); err != nil {
					t.Fatal(err)
				}
				db := database.GetDB()
				for key, value := range map[string]string{"tgBotToken": "browser-private-token", "ldapPassword": "browser-private-password"} {
					if err := db.Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
						t.Fatal(err)
					}
				}
				user := &model.User{Id: 2, Enable: true, Permissions: model.PermPanelSettings}
				if grant == "role" {
					user.Permissions = 0
					user.AdminRoleID = 7
					user.AdminRole = &model.AdminRole{ID: 7, Permissions: model.PermPanelSettings}
				}
				if grant == "owner" {
					user.IsPanelOwner = true
					user.IsSuperAdmin = true
				}
				r := gin.New()
				r.Use(func(c *gin.Context) {
					c.Set("LOGIN_USER_ROW", user)
					c.Set("base_path", "/test/")
					c.Set("I18n", func(_ weblocale.I18nType, key string, _ ...string) string {
						text, _ := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
						return text
					})
					c.Next()
				})
				controller.NewSettingController(r.Group("/test/panel"))
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
					if strings.HasPrefix(rq.URL.Path, "/test/assets/") {
						http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, rq)
						return
					}
					if rq.URL.Path == "/test/panel/settings" {
						w.Header().Set("Content-Type", "text/html; charset=utf-8")
						if err := tpl.ExecuteTemplate(w, "settings.html", map[string]any{"base_path": "/test/", "two_factor": false, "title": "menu.settings", "perms": map[string]bool{"panelOwner": user.IsPanelOwner, "superAdmin": user.IsSuperAdmin, "accessPanelSettings": true}}); err != nil {
							t.Error(err)
						}
						return
					}
					// Stub host subprocess reads only; settings reads and saves use the real controller and database.
					if rq.URL.Path == "/test/panel/setting/service" || rq.URL.Path == "/test/panel/setting/service/log" || strings.Contains(rq.URL.Path, "/ssl/") || strings.Contains(rq.URL.Path, "/api/") {
						w.Header().Set("Content-Type", "application/json")
						w.Write([]byte(`{"success":true,"obj":{}}`))
						return
					}
					r.ServeHTTP(w, rq)
				}))
				defer srv.Close()
				script, _ := filepath.Abs("../test_unit/delegated_settings_browser_test.cjs")
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "node", script)
				cmd.Env = append(os.Environ(), "SETTINGS_UI_URL="+srv.URL+"/test/panel/settings", "SETTINGS_UI_OWNER="+grant)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("browser: %v\n%s", err, out)
				} else {
					t.Log(string(out))
				}
				var setting model.Setting
				db.Where("key = ?", "tgBotToken").First(&setting)
				if setting.Value != "browser-updated-token" {
					t.Fatal("full settings save did not persist token")
				}
			})
		}
	}
}
