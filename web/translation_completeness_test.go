package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
	"text/template"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

func translationValues(prefix string, value map[string]any, out map[string]string) {
	for key, v := range value {
		if prefix != "" {
			key = prefix + "." + key
		}
		if nested, ok := v.(map[string]any); ok {
			translationValues(key, nested, out)
		} else if text, ok := v.(string); ok {
			out[key] = text
		}
	}
}

var translationVariable = regexp.MustCompile(`{{\s*\.([A-Za-z][A-Za-z0-9_]*)\s*}}`)

func translationVariables(text string) []string {
	result := []string{}
	for _, match := range translationVariable.FindAllStringSubmatch(text, -1) {
		result = append(result, match[1])
	}
	sort.Strings(result)
	return result
}

// Verify every shipped message rather than testing only English/Farsi fallback.
// Exact variables preserve Telegram/account data, and actual rendering catches
// broken template syntax that a key count or TOML parser would miss.
func TestAllLocaleMessagesRenderAndPreserveVariables(t *testing.T) {
	entries, err := fs.ReadDir(i18nFS, "translation")
	if err != nil {
		t.Fatal(err)
	}
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	reference := map[string]string{}
	catalogs := map[string]map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		data, err := i18nFS.ReadFile("translation/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]any
		if err = toml.Unmarshal(data, &parsed); err != nil {
			t.Fatal(err)
		}
		values := map[string]string{}
		translationValues("", parsed, values)
		catalogs[entry.Name()] = values
		if entry.Name() == "translate.en_US.toml" {
			reference = values
		}
		if _, err = bundle.ParseMessageFileBytes(data, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	for file, values := range catalogs {
		code := strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(file, "translate."), ".toml"), "_", "-")
		localizer := i18n.NewLocalizer(bundle, code)
		for key, english := range reference {
			translated, ok := values[key]
			if !ok {
				t.Errorf("%s lacks %s", file, key)
				continue
			}
			if strings.TrimSpace(english) != "" && strings.TrimSpace(translated) == "" {
				t.Errorf("%s/%s has an empty translation", file, key)
			}
			if key == "pages.inboundForm.camouflageForTheTlsRecordSizesOf" {
				for _, literal := range []string{"stop=N", "N=min-max"} {
					if !strings.Contains(translated, literal) {
						t.Errorf("%s/%s translated a configuration literal: %s", file, key, literal)
					}
				}
			}
			if strings.Contains(translated, "ZXQPH") || strings.Contains(translated, "ZXQID") {
				t.Errorf("%s/%s has an unreplaced translation token", file, key)
			}
			if fmt.Sprint(translationVariables(english)) != fmt.Sprint(translationVariables(translated)) {
				t.Errorf("%s/%s changed template variables", file, key)
			}
			vars := map[string]any{}
			for _, name := range translationVariables(english) {
				vars[name] = "TEST_" + name
			}
			tpl, err := template.New(key).Parse(translated)
			if err != nil {
				t.Errorf("%s/%s: %v", file, key, err)
				continue
			}
			if err = tpl.Execute(&bytes.Buffer{}, vars); err != nil {
				t.Errorf("%s/%s: %v", file, key, err)
			}
			id := strings.TrimSuffix(key, ".other")
			if _, err = localizer.Localize(&i18n.LocalizeConfig{MessageID: id, TemplateData: vars}); err != nil {
				t.Errorf("%s/%s cannot localize: %v", file, id, err)
			}
		}
	}
}

func flattenComponentLocale(prefix string, value any, out map[string]any) {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			flattenComponentLocale(name, item, out)
		}
	case []any:
		for index, item := range v {
			flattenComponentLocale(fmt.Sprintf("%s.%d", prefix, index), item, out)
		}
	default:
		out[prefix] = v
	}
}
func TestComponentLocaleCoverage(t *testing.T) {
	raw, err := assetsFS.ReadFile("assets/js/component-locales.js")
	if err != nil {
		t.Fatal(err)
	}
	jsonText := strings.TrimSuffix(strings.TrimSpace(strings.SplitN(string(raw), " = ", 2)[1]), ";")
	var locales map[string]map[string]any
	if err = json.Unmarshal([]byte(jsonText), &locales); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(i18nFS, "translation")
	if err != nil {
		t.Fatal(err)
	}
	reference := map[string]any{}
	flattenComponentLocale("", locales["en-US"], reference)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		code := strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "translate."), ".toml"), "_", "-")
		locale, ok := locales[code]
		if !ok {
			t.Errorf("missing component language %s", code)
			continue
		}
		values := map[string]any{}
		flattenComponentLocale("", locale, values)
		for key, want := range reference {
			got, ok := values[key]
			if !ok || fmt.Sprintf("%T", got) != fmt.Sprintf("%T", want) {
				t.Errorf("%s/%s missing or wrong type", code, key)
			}
		}
		// Transfer renders titles as an array, not an object keyed by index.
		if _, ok := locale["Transfer"].(map[string]any)["titles"].([]any); !ok {
			t.Errorf("%s Transfer.titles must be an array", code)
		}
	}
}

// References are checked against the real embedded catalog, so a new label cannot
// silently degrade to a raw message ID despite all locales having equal counts.
func TestTranslationTemplateReferencesAndConfigurationLiterals(t *testing.T) {
	raw, err := i18nFS.ReadFile("translation/translate.en_US.toml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := toml.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	reference := map[string]string{}
	translationValues("", parsed, reference)
	pattern := regexp.MustCompile(`\bi18n\s+"([A-Za-z][A-Za-z0-9_.]*)"`)
	if err := fs.WalkDir(htmlFS, "html", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := htmlFS.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range pattern.FindAllStringSubmatch(string(raw), -1) {
			if _, ok := reference[m[1]]; !ok {
				if _, ok := reference[m[1]+".other"]; !ok {
					t.Errorf("%s references missing message %s", path, m[1])
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(i18nFS, "translation")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		raw, err := i18nFS.ReadFile("translation/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]any
		if err := toml.Unmarshal(raw, &parsed); err != nil {
			t.Fatal(err)
		}
		values := map[string]string{}
		translationValues("", parsed, values)
		for key, english := range reference {
			var literals []string
			if strings.HasPrefix(key, "pages.interface.default") {
				for _, literal := range []string{"(path)", "(body)", "(queryInHeader)", "(repeat-x)", "(POST)"} {
					if strings.Contains(english, literal) {
						literals = append(literals, literal)
					}
				}
			}
			if key == "pages.interface.autoManagedContiguousBlockUDPShownTCPMirrors" {
				literals = []string{"10.2", "10.3"}
			}
			for _, literal := range literals {
				if !strings.Contains(values[key], literal) {
					t.Errorf("%s/%s changed literal %s", entry.Name(), key, literal)
				}
			}
		}
	}
}
