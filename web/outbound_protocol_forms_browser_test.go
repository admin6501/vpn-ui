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

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

func TestOutboundProtocolFormsBrowserRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable Chromium outbound table checks")
	}
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, code := range []string{"en_US", "fa_IR", "ar_EG", "ru_RU"} {
		if _, err := bundle.LoadMessageFile("translation/translate." + code + ".toml"); err != nil {
			t.Fatal(err)
		}
	}
	for _, lang := range []string{"en-US", "fa-IR", "ar-EG", "ru-RU"} {
		t.Run(lang, func(t *testing.T) {
			localizer := i18n.NewLocalizer(bundle, lang)
			tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, args ...string) (string, error) {
				vars := map[string]any{}
				for _, arg := range args {
					if k, v, ok := strings.Cut(arg, "=="); ok {
						vars[k] = v
					}
				}
				return localizer.Localize(&i18n.LocalizeConfig{MessageID: key, TemplateData: vars})
			}})
			if err != nil {
				t.Fatal(err)
			}
			var page bytes.Buffer
			if err := tpl.ExecuteTemplate(&page, "xray.html", map[string]any{"lang": lang, "base_path": "/test/", "title": "menu.xray", "perms": map[string]bool{"panelOwner": true, "superAdmin": true, "accessOverview": true, "xraySettings": true}}); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/test/assets/") {
					http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(page.Bytes())
			}))
			defer server.Close()
			script, _ := filepath.Abs("../test_unit/outbound_protocol_forms_browser_test.cjs")
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "node", script)
			cmd.Env = append(os.Environ(), "OUTBOUND_TABLE_URL="+server.URL+"/test/", "NODE_UI_LANG="+lang)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("browser: %v\n%s", err, output)
			} else {
				t.Log(string(output))
			}
		})
	}
}
