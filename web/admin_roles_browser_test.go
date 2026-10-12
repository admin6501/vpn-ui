package web

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/controller"
	weblocale "github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminRolesBrowserRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable Chromium browser checks")
	}
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	langs := []string{"en_US", "fa_IR", "ar_EG", "es_ES", "id_ID", "ja_JP", "pt_BR", "ru_RU", "tr_TR", "uk_UA", "vi_VN", "zh_CN", "zh_TW"}
	for _, lang := range langs {
		if _, err := bundle.LoadMessageFile("translation/translate." + lang + ".toml"); err != nil {
			t.Fatal(err)
		}
	}
	for _, lang := range langs {
		t.Run(lang, func(t *testing.T) {
			localizer := i18n.NewLocalizer(bundle, strings.ReplaceAll(lang, "_", "-"))
			tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, args ...string) (string, error) {
				return localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
			}})
			if err != nil {
				t.Fatal(err)
			}
			if err := database.InitDB(filepath.Join(t.TempDir(), "admin-browser.db")); err != nil {
				t.Fatal(err)
			}
			db := database.GetDB()
			var owner model.User
			db.Where("is_panel_owner = ?", true).First(&owner)
			current := func() *model.User {
				var u model.User
				db.First(&u, owner.Id)
				if u.AdminRoleID > 0 {
					var role model.AdminRole
					db.First(&role, u.AdminRoleID)
					u.AdminRole = &role
				}
				return &u
			}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("LOGIN_USER_ROW", current())
				c.Set("base_path", "/test/")
				c.Set("I18n", func(_ weblocale.I18nType, key string, args ...string) string {
					value, _ := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
					return value
				})
				c.Next()
			})
			controller.NewAdminController(router.Group("/test/panel"))
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/test/assets/") {
					http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, r)
					return
				}
				if r.URL.Path == "/test/panel/" {
					http.Redirect(w, r, "/test/panel/admins", 302)
					return
				}
				if r.URL.Path == "/test/panel/admins" {
					u := current()
					perms := map[string]bool{"panelOwner": u.IsPanelOwner, "superAdmin": u.IsSuperAdmin, "manageAdmins": u.CanManageAdmins()}
					for _, d := range model.AllPermissions {
						perms[d.Slug] = u.Can(d.Bit)
					}
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					if err := tpl.ExecuteTemplate(w, "admins.html", map[string]any{"base_path": "/test/", "title": "menu.admins", "me": u.Id, "perms": perms}); err != nil {
						t.Errorf("template render: %v", err)
					}
					return
				}
				router.ServeHTTP(w, r)
			}))
			defer srv.Close()
			script, _ := filepath.Abs("../test_unit/admin_roles_browser_test.cjs")
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "node", script)
			cmd.Env = append(os.Environ(), "ADMIN_UI_URL="+srv.URL+"/test/panel/admins", "NODE_UI_LANG="+strings.ReplaceAll(lang, "_", "-"))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("admin browser failed: %v\n%s", err, out)
			}
		})
	}
}
