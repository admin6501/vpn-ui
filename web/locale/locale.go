// Package locale provides internationalization (i18n) support for the vpn-ui web panel,
// including translation loading, localization, and middleware for web and bot interfaces.
package locale

import (
	"io/fs"
	"os"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v2/logger"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

var (
	localizersMu     sync.RWMutex
	i18nBundle       *i18n.Bundle
	LocalizerWeb     *i18n.Localizer
	LocalizerBot     *i18n.Localizer
	localizerDefault *i18n.Localizer // always English; fallback for keys missing in the active locale
)

// I18nType represents the type of interface for internationalization.
type I18nType string

const (
	Bot I18nType = "bot" // Bot interface type
	Web I18nType = "web" // Web interface type
)

// SettingService interface defines methods for accessing locale settings.
type SettingService interface {
	GetTgLang() (string, error)
}

// InitLocalizer initializes the internationalization system with embedded translation files.
func InitLocalizer(i18nFS fs.FS, settingService SettingService) error {
	// Build privately: an active bot/web request must never see a half-loaded bundle.
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if err := parseTranslationFiles(i18nFS, bundle); err != nil {
		return err
	}
	botLang, err := settingService.GetTgLang()
	if err != nil {
		return err
	}
	botLang = strings.ReplaceAll(botLang, "_", "-")
	if _, err := language.Parse(botLang); err != nil {
		botLang = "en-US"
	}
	fallback := i18n.NewLocalizer(bundle, "en-US")
	botLocalizer := i18n.NewLocalizer(bundle, botLang)
	localizersMu.Lock()
	i18nBundle = bundle
	localizerDefault = fallback
	LocalizerBot = botLocalizer
	localizersMu.Unlock()
	return nil
}

// createTemplateData creates a template data map from parameters with optional separator.
func createTemplateData(params []string, separator ...string) map[string]any {
	var sep string = "=="
	if len(separator) > 0 {
		sep = separator[0]
	}

	templateData := make(map[string]any)
	for _, param := range params {
		parts := strings.SplitN(param, sep, 2)
		if len(parts) == 2 && parts[0] != "" {
			templateData[parts[0]] = parts[1]
		}
	}

	return templateData
}

// I18n retrieves a localized message for the given key and type.
// It supports both bot and web contexts, with optional template parameters.
// Returns the localized message or an empty string if localization fails.
func I18n(i18nType I18nType, key string, params ...string) string {
	localizersMu.RLock()
	var localizer *i18n.Localizer
	fallback := localizerDefault

	switch i18nType {
	case "bot":
		localizer = LocalizerBot
	case "web":
		localizer = LocalizerWeb
	default:
		localizersMu.RUnlock()
		logger.Errorf("Invalid type for I18n: %s", i18nType)
		return ""
	}

	localizersMu.RUnlock()
	return localize(localizer, fallback, key, params...)
}

func localize(localizer, fallback *i18n.Localizer, key string, params ...string) string {
	templateData := createTemplateData(params)

	if localizer == nil {
		// Fallback to key if localizer not ready; prevents nil panic on pages like sub
		return key
	}

	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: templateData,
	})
	if err != nil {
		// Key missing/failed in the active locale. Fall back to English so a
		// string that exists only in en_US shows readable text instead of a
		// blank (this is the common case for not-yet-translated keys, so it is
		// not logged). Only if English also fails do we surface the key itself.
		if fallback != nil {
			if enMsg, enErr := fallback.Localize(&i18n.LocalizeConfig{
				MessageID:    key,
				TemplateData: templateData,
			}); enErr == nil {
				return enMsg
			}
		}
		logger.Errorf("Failed to localize message: %v", err)
		return key
	}

	return msg
}

// LocalizerMiddleware returns a Gin middleware that sets up localization for web requests.
// It determines the user's language from cookies or Accept-Language header,
// creates a localizer instance, and stores it in the Gin context for use in handlers.
// Also provides the I18n function in the context for template rendering.
func LocalizerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		localizersMu.Lock()
		// Ensure bundle is initialized so creating a Localizer won't panic
		if i18nBundle == nil {
			i18nBundle = i18n.NewBundle(language.MustParse("en-US"))
			i18nBundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
			// Try lazy-load from disk when running sub server without InitLocalizer
			if err := loadTranslationsFromDisk(i18nBundle); err != nil {
				logger.Warning("i18n lazy load failed:", err)
			}
			localizerDefault = i18n.NewLocalizer(i18nBundle, "en-US")
		}
		var lang string

		if cookie, err := c.Request.Cookie("lang"); err == nil {
			lang = cookie.Value
		} else {
			lang = c.GetHeader("Accept-Language")
		}

		LocalizerWeb = i18n.NewLocalizer(i18nBundle, lang)

		localizer := LocalizerWeb
		fallback := localizerDefault
		localizersMu.Unlock()
		c.Set("localizer", localizer)
		c.Set("I18n", func(kind I18nType, key string, params ...string) string {
			if kind == Web {
				return localize(localizer, fallback, key, params...)
			}
			return I18n(kind, key, params...)
		})
		c.Next()
	}
}

// loadTranslationsFromDisk attempts to load translation files from "web/translation" using the local filesystem.
func loadTranslationsFromDisk(bundle *i18n.Bundle) error {
	root := os.DirFS("web")
	return fs.WalkDir(root, "translation", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		_, err = bundle.ParseMessageFileBytes(data, path)
		return err
	})
}

// parseTranslationFiles parses embedded translation files and adds them to the i18n bundle.
func parseTranslationFiles(i18nFS fs.FS, i18nBundle *i18n.Bundle) error {
	err := fs.WalkDir(i18nFS, "translation",
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			data, err := fs.ReadFile(i18nFS, path)
			if err != nil {
				return err
			}

			_, err = i18nBundle.ParseMessageFileBytes(data, path)
			return err
		})
	if err != nil {
		return err
	}

	return nil
}
