package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

func TestPersianInboundFormsRender(t *testing.T) {
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range []string{"en_US", "fa_IR"} {
		if _, err := bundle.LoadMessageFile("translation/translate." + lang + ".toml"); err != nil {
			t.Fatal(err)
		}
	}
	localizer := i18n.NewLocalizer(bundle, "fa-IR")
	tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, args ...string) (string, error) {
		return localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"form/inbound", "form/inbound/legacy", "modals/inboundModal"} {
		var out bytes.Buffer
		if err := tpl.ExecuteTemplate(&out, name, map[string]any{"base_path": "/test/"}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(out.String(), "ZgotmplZ") || strings.Contains(out.String(), "pages.inboundForm.") {
			t.Fatalf("%s: invalid translated output", name)
		}
		if !strings.Contains(out.String(), "ورودی") {
			t.Fatalf("%s: Persian labels missing", name)
		}
	}
}
