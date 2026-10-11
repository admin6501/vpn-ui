package web

import (
	"io/fs"
	"sort"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// TestTranslationsAreValidToml unmarshals every embedded translation file so a
// malformed key (e.g. a bad bulk-ops i18n insertion) fails here instead of silently
// breaking i18n at runtime.
func TestTranslationsAreValidToml(t *testing.T) {
	entries, err := fs.ReadDir(i18nFS, "translation")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := i18nFS.ReadFile("translation/" + e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var m map[string]any
		if err := toml.Unmarshal(data, &m); err != nil {
			t.Errorf("%s: invalid TOML: %v", e.Name(), err)
		}
	}
}

// flattenKeys collapses nested TOML tables into dotted keys (e.g. "pages.core.title").
func flattenKeys(prefix string, m map[string]any, out map[string]bool) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			flattenKeys(key, sub, out)
		} else {
			out[key] = true
		}
	}
}

func loadTranslationKeys(t *testing.T, name string) map[string]bool {
	t.Helper()
	data, err := i18nFS.ReadFile("translation/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var m map[string]any
	if err := toml.Unmarshal(data, &m); err != nil {
		t.Fatalf("%s: invalid TOML: %v", name, err)
	}
	keys := make(map[string]bool)
	flattenKeys("", m, keys)
	return keys
}

// TestTranslationKeyParity requires complete key coverage in every shipped locale.
// No English-only exceptions: new features need translations before release.
func TestTranslationKeyParity(t *testing.T) {
	const ref = "translate.en_US.toml"
	refKeys := loadTranslationKeys(t, ref)

	entries, err := fs.ReadDir(i18nFS, "translation")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == ref {
			continue
		}
		locKeys := loadTranslationKeys(t, e.Name())
		var newlyMissing []string
		for k := range refKeys {
			if !locKeys[k] {
				newlyMissing = append(newlyMissing, k)
			}
		}
		if len(newlyMissing) > 0 {
			sort.Strings(newlyMissing)
			t.Errorf("%s: %d en_US key(s) missing (translate them before release): %v",
				e.Name(), len(newlyMissing), newlyMissing)
		}
	}
}
