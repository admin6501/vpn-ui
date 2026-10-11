// Provide the component library's language to every page and independent modal.
// Locale changes already reload the panel, so no mutable global language state
// or asynchronous asset fetch is required before the first render.
(function () {
    const selected = LanguageManager.getLanguage();
    const locale = PanelComponentLocales[selected] || PanelComponentLocales['en-US'];
    Vue.mixin({
        provide() {
            // Nested components inherit their root's context. An explicit
            // component locale/provider continues to override this default.
            return this.$parent ? {} : { localeData: { antLocale: { ...locale, exist: true } } };
        },
    });
    // Static confirmation dialogs mount outside the root component tree.
    ['confirm', 'info', 'success', 'error', 'warning'].forEach(method => {
        const original = antd.Modal[method];
        if (typeof original !== 'function') return;
        const localized = options => original({
            okText: method === 'confirm' ? locale.Modal.okText : (locale.Modal.justOkText || locale.Modal.okText),
            cancelText: locale.Modal.cancelText,
            ...options,
        });
        antd.Modal[method] = localized;
        Vue.prototype['$' + method] = localized;
    });
})();
