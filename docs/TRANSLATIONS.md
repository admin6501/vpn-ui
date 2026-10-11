# Panel translations

The panel and Telegram bot ship thirteen languages: English, Persian, Arabic,
Spanish, Indonesian, Japanese, Brazilian Portuguese, Russian, Turkish, Ukrainian,
Vietnamese, Simplified Chinese and Traditional Chinese.

Every language in `web/translation` must contain the full English key set.
New features must include their labels, descriptions, prompts and messages in
all languages. There is no list of deferred English-only keys in the tests.
Protocol names, configuration literals, file paths and `{{ .Variable }}` message
parameters must retain their meaning and spelling.

Ant Design Vue controls use the same selected language as the panel, including
confirmation dialogs, empty states, pagination and date/time picker controls.
The component and calendar language assets are bundled locally. Registering
calendar locales preserves Moment's default locale so wire date formatting
continues to use the existing format.

Run the catalog and rendering checks:

```bash
go test ./web -run 'TestTranslationKeyParity|TestAllLocaleMessagesRenderAndPreserveVariables|TestComponentLocaleCoverage|TestTranslationsAreValidToml' -count=1
```

The release workflow also opens the real node page and shared graphical Xray
editor in Chromium for every supported language. It checks translated component
controls, node creation, configuration editing and scoped runtime commands.

برای افزودن قابلیت جدید، ترجمهٔ برچسب‌ها، توضیحات و پیام‌ها را در هر ۱۳ فایل
کامل کنید. متغیرهای پیام، نام پروتکل‌ها و عبارت‌های پیکربندی نباید تغییر کنند.
آزمون بالا کامل بودن کلیدها و اجرای قالب تمام پیام‌های پنل و ربات را بررسی می‌کند.
