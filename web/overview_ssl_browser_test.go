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

func TestOverviewSSLBrowserRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable browser checks with RUN_NODE_UI_BROWSER=1")
	}
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range []string{"en_US", "fa_IR"} {
		if _, err := bundle.LoadMessageFile("translation/translate." + lang + ".toml"); err != nil {
			t.Fatal(err)
		}
	}
	for _, lang := range []string{"en-US", "fa-IR"} {
		t.Run(lang, func(t *testing.T) {
			localizer := i18n.NewLocalizer(bundle, lang)
			tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, args ...string) (string, error) {
				return localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
			}})
			if err != nil {
				t.Fatal(err)
			}
			data := map[string]any{"base_path": "/test/", "title": "menu.settings", "perms": map[string]bool{"panelOwner": true, "superAdmin": true, "accessOverview": true, "accessInbounds": true, "accessCoreSettings": true, "panelSettings": true}}
			pages := map[string][]byte{}
			for _, name := range []string{"index.html", "settings.html", "clients.html", "inbounds.html"} {
				var out bytes.Buffer
				if err := tpl.ExecuteTemplate(&out, name, data); err != nil {
					t.Fatal(err)
				}
				pages[name] = out.Bytes()
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasPrefix(r.URL.Path, "/test/assets/"):
					http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, r)
				case r.URL.Path == "/test/panel/api/clients/groups":
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"success":true,"msg":"","obj":[]}`))
				case r.URL.Path == "/test/panel/overview":
					w.Header().Set("Content-Type", "text/html")
					w.Write(pages["index.html"])
				case r.URL.Path == "/test/panel/clients":
					w.Header().Set("Content-Type", "text/html")
					w.Write(pages["clients.html"])
				case r.URL.Path == "/test/panel/inbounds":
					w.Header().Set("Content-Type", "text/html")
					w.Write(pages["inbounds.html"])
				case r.URL.Path == "/test/panel/settings":
					w.Header().Set("Content-Type", "text/html")
					w.Write(pages["settings.html"])
				default:
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"success":false,"msg":"","obj":null}`))
				}
			}))
			defer server.Close()
			script, _ := filepath.Abs("../test_unit/overview_ssl_browser_test.cjs")
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "node", script)
			cmd.Env = append(os.Environ(), "OVERVIEW_SSL_URL="+server.URL+"/test/panel/", "NODE_UI_LANG="+lang)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("browser failed: %v\n%s", err, out)
			} else {
				t.Log(string(out))
			}
			liveScript, _ := filepath.Abs("../test_unit/live_status_browser_test.cjs")
			liveCmd := exec.CommandContext(ctx, "node", liveScript)
			liveCmd.Env = cmd.Env
			if out, err := liveCmd.CombinedOutput(); err != nil {
				t.Fatalf("live status browser failed: %v\n%s", err, out)
			} else {
				t.Log(string(out))
			}
			refreshScript, _ := filepath.Abs("../test_unit/panel_refresh_browser_test.cjs")
			refreshCmd := exec.CommandContext(ctx, "node", refreshScript)
			refreshCmd.Env = cmd.Env
			if out, err := refreshCmd.CombinedOutput(); err != nil {
				t.Fatalf("panel refresh browser failed: %v\n%s", err, out)
			} else {
				t.Log(string(out))
			}
			restoreScript, _ := filepath.Abs("../test_unit/restore_browser_test.cjs")
			restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer restoreCancel()
			restoreCmd := exec.CommandContext(restoreCtx, "node", restoreScript)
			restoreCmd.Env = cmd.Env
			if out, err := restoreCmd.CombinedOutput(); err != nil {
				t.Fatalf("restore browser failed: %v\n%s", err, out)
			} else {
				t.Log(string(out))
			}

		})
	}
}
