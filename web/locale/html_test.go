package locale

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
)

func TestHTMLLocaleConcurrentPanelAndSubscriptionRequests(t *testing.T) {
	if err := InitLocalizer(os.DirFS(".."), botLanguage("ja-JP")); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	tpl := template.Must(template.New("page").Funcs(template.FuncMap{"i18n": func(key string) string { return I18n(Web, key) }}).
		Parse(`{{.lang}}|{{i18n "menu.clients"}}|{{i18n "subscription.status"}}|{{i18n "menu.clients"}}`))
	renderer, err := NewHTMLRender(render.HTMLProduction{Template: tpl})
	if err != nil {
		t.Fatal(err)
	}
	ready, release := make(chan struct{}, len(languageCodes)*2), make(chan struct{})
	router := gin.New()
	router.HTMLRender = renderer
	router.Use(LocalizerMiddleware())
	router.GET("/:page", func(c *gin.Context) {
		ready <- struct{}{}
		<-release
		c.HTML(200, "page", gin.H{"lang": RequestLanguage(c)})
	})
	expected := map[string]string{}
	for _, code := range languageCodes {
		localizersMu.RLock()
		loc, fb := webLocalizers[code], localizerDefault
		localizersMu.RUnlock()
		users, status := localize(loc, fb, "menu.clients"), localize(loc, fb, "subscription.status")
		expected[code] = code + "|" + users + "|" + status + "|" + users
	}
	var wg sync.WaitGroup
	for _, page := range []string{"panel", "subscription"} {
		for _, code := range languageCodes {
			wg.Add(1)
			go func(page, code string) {
				defer wg.Done()
				req := httptest.NewRequest(http.MethodGet, "/"+page, nil)
				req.AddCookie(&http.Cookie{Name: "lang", Value: code})
				out := httptest.NewRecorder()
				router.ServeHTTP(out, req)
				if out.Code != 200 || out.Body.String() != expected[code] {
					t.Errorf("%s/%s: %q, want %q", page, code, out.Body.String(), expected[code])
				}
			}(page, code)
		}
	}
	for range len(languageCodes) * 2 {
		<-ready
	}
	close(release)
	wg.Wait()
}

func TestRequestLanguageCanonicalDialectsAndCookieFallback(t *testing.T) {
	for _, tc := range []struct{ cookie, header, want string }{
		{"fa_IR", "en-US", "fa-IR"}, {"pt-PT", "en-US", "pt-BR"}, {"zh-HK", "en-US", "zh-TW"},
		{"", "zh-Hant", "zh-TW"}, {"", "en-GB", "en-US"}, {"", "es-MX", "es-ES"},
		{"invalid_cookie", "ja-JP", "ja-JP"}, {"", "de-DE,fa-IR;q=0.9", "fa-IR"}, {"", "de-DE", "en-US"},
	} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.cookie != "" {
				c.Request.AddCookie(&http.Cookie{Name: "lang", Value: tc.cookie})
			}
			c.Request.Header.Set("Accept-Language", tc.header)
			if got := RequestLanguage(c); got != tc.want {
				t.Errorf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestHTMLLocaleDebugReloadKeepsRequestLanguage(t *testing.T) {
	if err := InitLocalizer(os.DirFS(".."), botLanguage("en-US")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "page.html")
	write := func(suffix string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(`{{i18n "menu.clients"}} `+suffix), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("first")
	r, err := NewHTMLRender(render.HTMLDebug{Files: []string{path}, Delims: render.Delims{Left: "{{", Right: "}}"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"first", "updated"} {
		write(suffix)
		out := httptest.NewRecorder()
		if err := r.Instance("page.html", gin.H{"lang": "fa-IR"}).Render(out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.Body.String(), suffix) || strings.Contains(out.Body.String(), "Users") {
			t.Fatal("debug renderer lost file update or request language:", out.Body.String())
		}
	}
}
