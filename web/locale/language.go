package locale

import (
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/language"
)

var languageCodes = []string{"en-US", "fa-IR", "ar-EG", "es-ES", "id-ID", "ja-JP", "pt-BR", "ru-RU", "tr-TR", "uk-UA", "vi-VN", "zh-CN", "zh-TW"}
var supportedMatcher = func() language.Matcher {
	tags := make([]language.Tag, len(languageCodes))
	for i, code := range languageCodes {
		tags[i] = language.MustParse(code)
	}
	return language.NewMatcher(tags)
}()

// LanguageCodes returns the same thirteen canonical locales as the UI and bot.
func LanguageCodes() []string { return append([]string(nil), languageCodes...) }

func resolveLanguage(value string) (string, bool) {
	tags, _, err := language.ParseAcceptLanguage(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	if err != nil || len(tags) == 0 {
		return "en-US", false
	}
	_, index, confidence := supportedMatcher.Match(tags...)
	return languageCodes[index], confidence != language.No
}

// RequestLanguage resolves dialects and invalid cookies once, before rendering.
func RequestLanguage(c *gin.Context) string {
	if code := c.GetString("language"); code != "" {
		return code
	}
	if cookie, err := c.Cookie("lang"); err == nil {
		if code, ok := resolveLanguage(cookie); ok {
			return code
		}
	}
	code, _ := resolveLanguage(c.GetHeader("Accept-Language"))
	return code
}
