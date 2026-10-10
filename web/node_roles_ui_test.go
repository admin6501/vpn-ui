package web

import (
	"bytes"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

func TestRepresentativeRolesPagesRenderBothLanguages(t *testing.T) {
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, locale := range []string{"en_US", "fa_IR"} {
		if _, err := bundle.LoadMessageFile("translation/translate." + locale + ".toml"); err != nil {
			t.Fatal(err)
		}
	}
	scripts := regexp.MustCompile(`(?s)<script[^>]*>(.*?)</script>`)
	for _, locale := range []string{"en-US", "fa-IR"} {
		localizer := i18n.NewLocalizer(bundle, locale)
		tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, args ...string) (string, error) {
			return localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
		}})
		if err != nil {
			t.Fatal(err)
		}
		for _, page := range []string{"resellers.html"} {
			var output bytes.Buffer
			data := map[string]any{"base_path": "/test/", "title": "pages.resellers.title", "perms": map[string]bool{"superAdmin": true, "manageResellers": true}}
			if err := tpl.ExecuteTemplate(&output, page, data); err != nil {
				t.Fatalf("%s %s: %v", locale, page, err)
			}
			html := output.String()
			if strings.Contains(html, "ZgotmplZ") {
				t.Fatal("unsafe template output")
			}
			if page == "resellers.html" && strings.Contains(html, "representativeRoleId: 3") {
				t.Fatal("hardcoded missing role restored")
			}
			if node, err := exec.LookPath("node"); err == nil {
				for i, script := range scripts.FindAllStringSubmatch(html, -1) {
					if strings.TrimSpace(script[1]) == "" {
						continue
					}
					path := filepath.Join(t.TempDir(), "script.js")
					if err := os.WriteFile(path, []byte(script[1]), 0600); err != nil {
						t.Fatal(err)
					}
					cmd := exec.Command(node, "--check", path)
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("%s %s script %d: %s", locale, page, i, out)
					}
				}
			}
		}
	}
}
