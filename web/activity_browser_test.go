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

func TestPanelActivityBrowserRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable Chromium activity checks")
	}
	bundle := i18n.NewBundle(language.AmericanEnglish)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if _, err := bundle.LoadMessageFile("translation/translate.en_US.toml"); err != nil {
		t.Fatal(err)
	}
	localizer := i18n.NewLocalizer(bundle, "en-US")
	tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, args ...string) (string, error) {
		vars := map[string]any{}
		for _, arg := range args {
			if name, value, ok := strings.Cut(arg, "=="); ok {
				vars[name] = value
			}
		}
		return localizer.Localize(&i18n.LocalizeConfig{MessageID: key, TemplateData: vars})
	}})
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string][]byte{}
	for _, mode := range []string{"logins", "renewals", "changes", "apps", "expired", "payments"} {
		var out bytes.Buffer
		name := "activity.html"
		if mode == "expired" {
			name = "clients.html"
		}
		if err := tpl.ExecuteTemplate(&out, name, map[string]any{
			"lang": "en-US", "base_path": "/test/", "title": "pages.activity." + mode, "me": 1,
			"activityMode": mode, "expiredOwned": mode == "expired",
			"perms": map[string]bool{"panelOwner": true, "superAdmin": true, "accessInbounds": true},
		}); err != nil {
			t.Fatal(err)
		}
		pages[mode] = out.Bytes()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/test/assets/") {
			http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, r)
			return
		}
		if body := pages[strings.TrimPrefix(r.URL.Path, "/test/panel/activity/")]; body != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"msg":"","obj":[]}`))
	}))
	defer server.Close()
	script, err := filepath.Abs("../test_unit/panel_activity_browser_test.cjs")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Env = append(os.Environ(), "PANEL_ACTIVITY_BASE="+server.URL+"/test/")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser: %v\n%s", err, output)
	}
}
