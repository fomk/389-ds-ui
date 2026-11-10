package pages

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The UI renders pages generically, so every text a page needs must exist in
// the complete locales (docs/i18n.md); missing ones would show raw keys.
func TestLocalesCoverPages(t *testing.T) {
	ps, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cols, err := LoadCollections()
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "ru"} {
		keys := loadLocale(t, lang)
		need := func(k, why string) {
			if !keys[k] {
				t.Errorf("%s.json: missing %s (%s)", lang, k, why)
			}
		}
		for _, p := range ps {
			need("pages."+p.ID+".title", "page title")
			need("pages._sections."+p.Section, "section of "+p.ID)
			for _, f := range p.Fields {
				prefix := "pages." + p.ID + ".fields." + f.Attr
				if f.Text != "" {
					prefix = "pages._fields." + f.Text
				}
				need(prefix+".label", p.ID)
				if f.Group != "" {
					need("pages._groups."+f.Group, p.ID+" "+f.Attr)
				}
				if f.Unit != "" {
					need("pages._units."+f.Unit, p.ID+" "+f.Attr)
				}
				for _, fl := range f.Flags {
					need("pages."+p.ID+".flags."+fl.Name, f.Attr)
				}
			}
		}
		for _, c := range cols {
			need("pages."+c.ID+".title", "collection title")
			for _, f := range c.Fields {
				need("pages."+c.ID+".fields."+f.Attr+".label", c.ID)
				if f.Group != "" {
					need("pages._groups."+f.Group, c.ID+" "+f.Attr)
				}
			}
		}
		for _, u := range []string{"B", "KB", "MB", "GB", "TB", "s", "min", "h", "d"} {
			need("pages._units."+u, "size/duration unit")
		}
	}
}

func loadLocale(t *testing.T, lang string) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "i18n", "locales", lang+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		if obj, ok := v.(map[string]any); ok {
			for k, c := range obj {
				walk(prefix+k+".", c)
			}
			return
		}
		out[prefix[:len(prefix)-1]] = true
	}
	walk("", m)
	return out
}
