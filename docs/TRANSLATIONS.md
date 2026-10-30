# Thirteen-language interface — v1.0.0

The first stable release ships the same complete catalog in thirteen locales:

| Code | Language | Installation guide |
| --- | --- | --- |
| en-US | English | [README](../README.md) |
| fa-IR | فارسی | [راهنما](../README_FA.md) |
| ar-EG | العربية | [الدليل](../README_AR.md) |
| es-ES | Español | [Guía](../README_ES.md) |
| id-ID | Bahasa Indonesia | [Panduan](../README_ID.md) |
| ja-JP | 日本語 | [ガイド](../README_JA.md) |
| pt-BR | Português (Brasil) | [Guia](../README_PT_BR.md) |
| ru-RU | Русский | [Руководство](../README_RU.md) |
| tr-TR | Türkçe | [Kılavuz](../README_TR.md) |
| uk-UA | Українська | [Посібник](../README_UK.md) |
| vi-VN | Tiếng Việt | [Hướng dẫn](../README_VI.md) |
| zh-CN | 简体中文 | [指南](../README_ZH.md) |
| zh-TW | 繁體中文 | [指南](../README_ZH_TW.md) |

## What follows the selected language

Panel pages, subscription browser presentation, forms, explanations, dialogs and shared component
controls use the selected web language. The built-in management Telegram bot uses the language configured in its
settings, not a different automatic language for every chat. Terminal installers/menu output is
English; the migration wizard offers Persian and English. Protocol identifiers, enum values,
configuration examples, file paths, app names, command names and raw backend diagnostics keep their
required technical syntax. Original third-party documentation retains its upstream language.

The middleware resolves a valid language cookie first, then `Accept-Language`; unknown languages
fall back to English. Regional aliases such as `fa_IR`, `pt-PT` and `zh-HK` resolve to supported
locales. Traditional Chinese includes `zh-Hant`, Hong Kong and Macao variants. The browser uses
the canonical server-rendered language and does not reload a correct first page merely to save a
cookie. Changing the selector explicitly reloads the page in the chosen language.

Each production HTML renderer has thirteen bounded template instances with language-specific
translation functions. Concurrent panel and subscription requests cannot change each other's
language through a mutable global localizer. Development templates still reload edited files.
Bot/catalog reloads swap complete localizers rather than exposing a half-loaded catalog.

Ant Design Vue dialogs, empty states, pagination and date/time pickers use the same language.
Component and calendar assets are bundled locally. Registering calendar locales preserves the
existing Moment default so network date formats do not change accidentally. The HTML `lang`
attribute describes the actual selected locale.

## Catalog requirements and checks

Every file in `web/translation` must contain the complete English key set. Add labels, tooltips,
alerts, validation, prompts and bot messages in every language; there is no deferred English-only
key list. Preserve `{{ .Variable }}` names, configuration literals such as `stop=N`/`N=min-max`,
address ranges, units and enum values. Translate a complete sentence rather than concatenating
separately translated grammar fragments. Put human translations in catalogs, not form enum values.

```bash
go test ./web -run 'TestTranslationsAreValidToml|TestTranslationKeyParity|TestAllLocaleMessagesRenderAndPreserveVariables|TestTranslationTemplateReferencesAndConfigurationLiterals|TestComponentLocaleCoverage' -count=1
go test -race ./web/locale ./web/service -run 'TestHTMLLocale|TestRequestLanguage|TestBotLocaleReload|TestTelegramAllLanguages' -count=1
node test_unit/language_selection_test.cjs
python3 test_unit/documentation_test.py
```

The browser checks in the [release workflow](../.github/workflows/release.yml) open Overview,
settings, Users, Inbounds, node creation/configuration and subscriptions in all thirteen locales.
They exercise real shipped Vue/components and fail on JavaScript errors. Catalog checks execute
every translated message, compare interpolation variables and detect unresolved translation tokens;
reference checks detect template keys missing from English. Mock Telegram tests send commands only
to a local test server. The thirteen READMEs share executable code blocks, release filenames and
language navigation; their relative links are checked too.

Automated completeness is not proof of perfect prose. Review terminology and technical meaning
when editing translations, especially bits versus bytes, reset versus renewal, disabled access,
TLS warnings and protocol-specific default values. For current operation see the
[panel guide](PANEL-GUIDE.md) or [راهنمای فارسی](PANEL-GUIDE.fa.md).

The separate [sales bot](../telegram-bot/README-server.txt) currently has a Persian interface; it does not share the panel bot language selector.
