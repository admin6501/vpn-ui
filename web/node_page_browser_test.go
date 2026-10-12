package web

import (
	"bytes"
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

// Exercise the rendered page and actual shipped scripts in Chromium, so missing
// component scripts/variables cannot slip through a JavaScript syntax check.
func TestNodePageBrowserRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable RUN_NODE_UI_BROWSER=1 with Playwright and a Chromium executable")
	}
	chrome := os.Getenv("NODE_UI_CHROME")
	if chrome == "" {
		for _, name := range []string{"chromium", "google-chrome", "chromium-browser"} {
			if path, err := exec.LookPath(name); err == nil {
				chrome = path
				break
			}
		}
	}
	if chrome == "" {
		t.Fatal("Chromium required for requested browser test")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, locale := range []string{"en_US", "fa_IR", "ar_EG", "es_ES", "id_ID", "ja_JP", "pt_BR", "ru_RU", "tr_TR", "uk_UA", "vi_VN", "zh_CN", "zh_TW"} {
		if _, err = bundle.LoadMessageFile("translation/translate." + locale + ".toml"); err != nil {
			t.Fatal(err)
		}
	}
	for _, locale := range []string{"en-US", "fa-IR", "ar-EG", "es-ES", "id-ID", "ja-JP", "pt-BR", "ru-RU", "tr-TR", "uk-UA", "vi-VN", "zh-CN", "zh-TW"} {
		t.Run(locale, func(t *testing.T) {
			localizer := i18n.NewLocalizer(bundle, locale)
			tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, args ...string) (string, error) {
				return localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
			}})
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			data := map[string]any{"base_path": "/test/", "title": "pages.nodes.title", "perms": map[string]bool{"superAdmin": false, "manageNodes": true, "accessOverview": true, "accessInbounds": true}}
			if err = tpl.ExecuteTemplate(&output, "nodes.html", data); err != nil {
				t.Fatal(err)
			}
			nodeData := map[string]any{"base_path": "/test/", "title": "pages.nodes.configuration", "node_id": 1, "node_name": "Germany", "perms": data["perms"]}
			if err = tpl.ExecuteTemplate(&bytes.Buffer{}, "xray.html", nodeData); err != nil {
				t.Fatal(err)
			}
			if err := database.InitDB(filepath.Join(t.TempDir(), "browser.db")); err != nil {
				t.Fatal(err)
			}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.SetHTMLTemplate(tpl)
			router.Use(func(c *gin.Context) {
				c.Set("LOGIN_USER_ROW", &model.User{Id: 1, Username: "test-admin", Permissions: model.PermManageNodes, Enable: true})
				c.Set("base_path", "/test/")
				c.Set("I18n", func(_ weblocale.I18nType, key string, args ...string) string { return key })
				c.Next()
			})
			controller.NewNodeController(router.Group("/test/panel"))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasPrefix(r.URL.Path, "/test/assets/"):
					http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, r)
				case r.URL.Path == "/test/panel/nodes/":
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.Write(output.Bytes())
				default:
					router.ServeHTTP(w, r)
				}
			}))
			defer server.Close()
			script, err := filepath.Abs("../test_unit/node_page_browser_test.cjs")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, node, script)
			cmd.Env = append(os.Environ(), "NODE_UI_URL="+server.URL+"/test/panel/nodes/", "NODE_UI_CHROME="+chrome, "NODE_UI_LANG="+locale)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("browser runtime failed: %v\n%s", err, out)
			} else {
				t.Log(string(out))
			}
		})
	}
}
