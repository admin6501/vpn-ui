package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRepresentativeOverviewHidesInboundManagementAndUpdates(t *testing.T) {
	tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, args ...string) (string, error) { return key, nil }})
	if err != nil {
		t.Fatal(err)
	}
	var page bytes.Buffer
	data := map[string]any{"base_path": "/test/", "title": "menu.dashboard", "perms": map[string]bool{"accessOverview": true, "accessInbounds": true, "accessInboundManagement": false, "checkUpdates": false}, "reseller": map[string]any{"isReseller": true, "allowanceBytes": 100 << 30, "spentBytes": 25 << 30, "availableBytes": 75 << 30}}
	if err = tpl.ExecuteTemplate(&page, "index.html", data); err != nil {
		t.Fatal(err)
	}
	body := strings.ReplaceAll(page.String(), `\/`, `/`)
	for _, forbidden := range []string{"@click=\"checkForUpdate(true)\"", "key: '/test/panel/inbounds'", `class="bo-tile bo-t-flow"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("representative page exposed %s", forbidden)
		}
	}
	for _, required := range []string{"data-testid=\"representative-quota\"", "quotaBalance.allowanceBytes", "quotaBalance.availableBytes", "key: '/test/panel/clients'"} {
		if !strings.Contains(body, required) {
			t.Fatalf("representative page missing %s", required)
		}
	}
	if os.Getenv("RUN_NODE_UI_BROWSER") == "1" {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/test/assets/") {
				http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/test/panel/" {
				w.Header().Set("Content-Type", "text/html")
				w.Write(page.Bytes())
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.Path, "/resellerBalance") {
				fmt.Fprint(w, `{"success":true,"obj":{"isReseller":true,"unlimited":false,"allowanceBytes":107374182400,"spentBytes":26843545600,"availableBytes":80530636800}}`)
				return
			}
			fmt.Fprint(w, `{"success":true,"obj":null}`)
		}))
		defer server.Close()
		cmd := exec.Command("node", "../test_unit/representative_overview_browser_test.cjs")
		cmd.Env = append(os.Environ(), "REPRESENTATIVE_OVERVIEW_URL="+server.URL+"/test/panel/")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("representative browser: %v\n%s", err, output)
		}
	}

}

func TestAdministratorSidebarKeepsGrantedInbounds(t *testing.T) {
	tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, args ...string) (string, error) { return key, nil }})
	if err != nil {
		t.Fatal(err)
	}
	// Render with the established capability map as well as the new map. A missing
	// optional UI flag must not hide a granted administrator navigation entry.
	for _, granted := range []bool{false, true} {
		var page bytes.Buffer
		data := map[string]any{"base_path": "/test/", "title": "menu.dashboard", "perms": map[string]bool{"accessInbounds": granted}, "reseller": map[string]any{"isReseller": false}}
		if err = tpl.ExecuteTemplate(&page, "index.html", data); err != nil {
			t.Fatal(err)
		}
		body := strings.ReplaceAll(page.String(), `\/`, `/`)
		if strings.Contains(body, "key: '/test/panel/inbounds'") != granted {
			t.Fatalf("admin sidebar did not honor inbound permission=%v", granted)
		}
	}
}
