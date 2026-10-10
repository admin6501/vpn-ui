package locale

import (
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

type botLanguage string

func (lang botLanguage) GetTgLang() (string, error) { return string(lang), nil }

func TestPersianBotLocalizationAndMalformedParameters(t *testing.T) {
	if err := InitLocalizer(os.DirFS(".."), botLanguage("fa_IR")); err != nil {
		t.Fatal(err)
	}
	text := I18n(Bot, "tgbot.commands.start", "Firstname==Ali", "malformed", "==empty")
	if !strings.Contains(text, "سلام") || !strings.Contains(text, "Ali") {
		t.Fatalf("invalid Persian greeting: %q", text)
	}
}

func TestBotLocaleReloadWhileWebRequestsContinue(t *testing.T) {
	if err := InitLocalizer(os.DirFS(".."), botLanguage("en-US")); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(LocalizerMiddleware())
	router.GET("/", func(c *gin.Context) {
		translate := c.MustGet("I18n").(func(I18nType, string, ...string) string)
		c.String(200, translate(Web, "pages.inboundForm.identity"))
	})
	var wg sync.WaitGroup
	for _, lang := range []string{"fa-IR", "en-US"} {
		wg.Go(func() {
			want := "Identity"
			if lang == "fa-IR" {
				want = "مشخصات"
			}
			for i := 0; i < 40; i++ {
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest("GET", "/", nil)
				request.Header.Set("Accept-Language", lang)
				router.ServeHTTP(recorder, request)
				if recorder.Body.String() != want {
					t.Errorf("request language %s leaked: %q", lang, recorder.Body.String())
					return
				}
				if text := I18n(Bot, "tgbot.commands.status"); text == "" || text == "tgbot.commands.status" {
					t.Errorf("bot lost messages during reload: %q", text)
					return
				}
			}
		})
	}
	for i := 0; i < 8; i++ {
		lang := botLanguage("fa-IR")
		if i%2 == 0 {
			lang = "en-US"
		}
		if err := InitLocalizer(os.DirFS(".."), lang); err != nil {
			t.Error(err)
		}
	}
	wg.Wait()
}
