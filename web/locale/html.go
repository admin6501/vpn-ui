package locale

import (
	"html/template"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
)

// Each compiled template closes over a fixed language, never LocalizerWeb, which
// another panel or subscription request can change halfway through rendering.
// Thirteen clones keep allocations bounded; debug templates still reload files.
type localizedHTML struct {
	base      render.HTMLRender
	templates map[string]*template.Template
	funcs     map[string]func(string, ...string) string
}

func NewHTMLRender(base render.HTMLRender) (render.HTMLRender, error) {
	r := &localizedHTML{base: base, templates: map[string]*template.Template{}, funcs: map[string]func(string, ...string) string{}}
	for _, code := range languageCodes {
		code := code
		translate := func(key string, params ...string) string {
			localizersMu.RLock()
			localizer, fallback := webLocalizers[code], localizerDefault
			localizersMu.RUnlock()
			return localize(localizer, fallback, key, params...)
		}
		r.funcs[code] = translate
		if production, ok := base.(render.HTMLProduction); ok {
			cloned, err := production.Template.Clone()
			if err != nil {
				return nil, err
			}
			r.templates[code] = cloned.Funcs(template.FuncMap{"i18n": translate})
		}
	}
	return r, nil
}

func (r *localizedHTML) Instance(name string, data any) render.Render {
	code := "en-US"
	if values, ok := data.(gin.H); ok {
		if value, ok := values["lang"].(string); ok {
			code, _ = resolveLanguage(value)
		}
	} else if values, ok := data.(map[string]any); ok {
		if value, ok := values["lang"].(string); ok {
			code, _ = resolveLanguage(value)
		}
	}
	if tpl := r.templates[code]; tpl != nil {
		return render.HTML{Template: tpl, Name: name, Data: data}
	}
	if debug, ok := r.base.(render.HTMLDebug); ok {
		funcs := template.FuncMap{}
		for key, value := range debug.FuncMap {
			funcs[key] = value
		}
		funcs["i18n"] = r.funcs[code]
		debug.FuncMap = funcs
		return debug.Instance(name, data)
	}
	return r.base.Instance(name, data)
}
