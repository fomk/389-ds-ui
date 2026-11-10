// Package pages implements declarative settings pages: YAML descriptions of
// one LDAP entry (usually under cn=config) and its attributes. The API reads
// and writes them generically, the UI renders them generically, and texts live
// in the UI locale files (pages.<id>.*), so adding a page needs no Go code.
package pages

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"go.yaml.in/yaml/v3"
)

//go:embed defs/*.yaml
var defs embed.FS

// Field types.
const (
	TypeString   = "string"
	TypeInt      = "int"
	TypeBool     = "bool"
	TypeEnum     = "enum"
	TypeDN       = "dn"
	TypeList     = "list"     // multi-valued strings
	TypeDuration = "duration" // integer seconds
	TypeSize     = "size"     // integer bytes
	TypeFlags    = "flags"    // integer bit mask with named bits
)

var types = map[string]bool{TypeString: true, TypeInt: true, TypeBool: true, TypeEnum: true, TypeDN: true,
	TypeList: true, TypeDuration: true, TypeSize: true, TypeFlags: true}

// OptionsFrom fills enum values from the server: the attr of every entry one
// level below base.
type OptionsFrom struct {
	Base string `yaml:"base" json:"-"`
	Attr string `yaml:"attr" json:"-"`
}

// PageFlag is one named bit of a flags field.
type PageFlag struct {
	Name  string `yaml:"name" json:"name"`
	Value int64  `yaml:"value" json:"value"`
}

// PageField is one attribute of a page.
type PageField struct {
	Attr        string       `yaml:"attr" json:"attr"`
	Type        string       `yaml:"type" json:"type" enum:"string,int,bool,enum,dn,list,duration,size,flags"`
	Group       string       `yaml:"group,omitempty" json:"group,omitempty" doc:"Heading the field is shown under (pages._groups.<group>)"`
	Text        string       `yaml:"text,omitempty" json:"text,omitempty" doc:"Shared text key (pages._fields.<text>) instead of pages.<id>.fields.<attr>"`
	Min         *int64       `yaml:"min,omitempty" json:"min,omitempty"`
	Max         *int64       `yaml:"max,omitempty" json:"max,omitempty"`
	Unit        string       `yaml:"unit,omitempty" json:"unit,omitempty" doc:"Unit of an int field, shown next to it (MB, ms, …)"`
	Values      []string     `yaml:"values,omitempty" json:"values,omitempty" doc:"Enum values"`
	Required    bool         `yaml:"required,omitempty" json:"required,omitempty" doc:"Collections: the value cannot be empty"`
	Bool        []string     `yaml:"bool,omitempty" json:"bool,omitempty" doc:"Literal for true and false; default on/off"`
	Flags       []PageFlag   `yaml:"flags,omitempty" json:"flags,omitempty" doc:"Named bits of a flags field"`
	OptionsFrom *OptionsFrom `yaml:"optionsFrom,omitempty" json:"-"`
	Restart     bool         `yaml:"restart,omitempty" json:"restart,omitempty" doc:"Takes effect after a server restart"`
	Danger      bool         `yaml:"danger,omitempty" json:"danger,omitempty" doc:"A wrong value can cut ds-ui off from the server"`
	ReadOnly    bool         `yaml:"readOnly,omitempty" json:"readOnly,omitempty"`
}

// Page describes one settings page.
type Page struct {
	ID      string            `yaml:"id" json:"id"`
	Section string            `yaml:"section" json:"section" doc:"Menu section: server, database, logging, security, …"`
	Order   int               `yaml:"order" json:"order"`
	DN      string            `yaml:"dn" json:"dn"`
	When    map[string]string `yaml:"when,omitempty" json:"-" doc:"Conditions such as dbImpl: mdb"`
	Vars    map[string]string `yaml:"vars,omitempty" json:"-" doc:"Substitutions for {name} in id, dn and attributes"`
	Use     []string          `yaml:"use,omitempty" json:"-" doc:"Fieldsets of the file appended to fields"`
	Plugin  string            `yaml:"plugin,omitempty" json:"plugin,omitempty" doc:"cn of the plug-in this page configures; shown on the Plugins page"`
	Sparse  bool              `yaml:"sparse,omitempty" json:"sparse,omitempty" doc:"Attributes may be absent until set (plug-in configuration, local policies); the UI shows an explicit default state"`
	// EntryClass makes the page edit any entry of this object class, named
	// by the caller (?dn=), instead of the fixed DN.
	EntryClass string      `yaml:"entryClass,omitempty" json:"entryClass,omitempty" doc:"The page edits the entry given as ?dn=, which must have this object class"`
	Fields     []PageField `yaml:"fields" json:"fields"`
}

// file is one YAML definition file.
type file struct {
	Fieldsets   map[string][]PageField `yaml:"fieldsets"`
	Pages       []Page                 `yaml:"pages"`
	Collections []Collection           `yaml:"collections"`
}

var idRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Load reads and validates the embedded page definitions.
func Load() ([]*Page, error) {
	p, _, err := loadAll(defs)
	return p, err
}

// LoadCollections reads and validates the embedded collection definitions.
func LoadCollections() ([]*Collection, error) {
	_, c, err := loadAll(defs)
	return c, err
}

func load(fsys fs.FS) ([]*Page, error) {
	p, _, err := loadAll(fsys)
	return p, err
}

func loadAll(fsys fs.FS) ([]*Page, []*Collection, error) {
	files, err := fs.Glob(fsys, "defs/*.yaml")
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(files)
	var out []*Page
	var cols []*Collection
	seen := map[string]bool{}
	for _, name := range files {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, nil, err
		}
		var f file
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if err := dec.Decode(&f); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path.Base(name), err)
		}
		for i := range f.Pages {
			p := &f.Pages[i]
			for _, u := range p.Use {
				set, ok := f.Fieldsets[u]
				if !ok {
					return nil, nil, fmt.Errorf("%s: page %s: unknown fieldset %q", path.Base(name), p.ID, u)
				}
				p.Fields = append(p.Fields, slices.Clone(set)...)
			}
			p.expand()
			if err := p.validate(); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", path.Base(name), err)
			}
			if seen[p.ID] {
				return nil, nil, fmt.Errorf("%s: duplicate page id %q", path.Base(name), p.ID)
			}
			seen[p.ID] = true
			out = append(out, p)
		}
		for i := range f.Collections {
			c := &f.Collections[i]
			for _, u := range c.Use {
				set, ok := f.Fieldsets[u]
				if !ok {
					return nil, nil, fmt.Errorf("%s: collection %s: unknown fieldset %q", path.Base(name), c.ID, u)
				}
				c.Fields = append(c.Fields, slices.Clone(set)...)
			}
			if err := c.validate(); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", path.Base(name), err)
			}
			if seen["collection:"+c.ID] {
				return nil, nil, fmt.Errorf("%s: duplicate collection id %q", path.Base(name), c.ID)
			}
			seen["collection:"+c.ID] = true
			cols = append(cols, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Section != out[j].Section {
			return out[i].Section < out[j].Section
		}
		return out[i].Order < out[j].Order
	})
	// Nested collections must name an existing one.
	for _, c := range cols {
		if c.Under != "" && !slices.ContainsFunc(cols, func(x *Collection) bool { return x.ID == c.Under }) {
			return nil, nil, fmt.Errorf("collection %s: unknown parent collection %q", c.ID, c.Under)
		}
	}
	return out, cols, nil
}

// expand substitutes {var} in id, dn and attribute names.
func (p *Page) expand() {
	if len(p.Vars) == 0 {
		return
	}
	r := make([]string, 0, 2*len(p.Vars))
	for k, v := range p.Vars {
		r = append(r, "{"+k+"}", v)
	}
	rep := strings.NewReplacer(r...)
	p.ID, p.DN = rep.Replace(p.ID), rep.Replace(p.DN)
	for i := range p.Fields {
		p.Fields[i].Attr = rep.Replace(p.Fields[i].Attr)
	}
}

func (p *Page) validate() error {
	if !idRE.MatchString(p.ID) {
		return fmt.Errorf("invalid page id %q", p.ID)
	}
	if p.Section == "" {
		return fmt.Errorf("page %s: section is empty", p.ID)
	}
	switch {
	case p.EntryClass != "" && p.DN != "":
		return fmt.Errorf("page %s: dn and entryClass exclude each other", p.ID)
	case p.EntryClass == "":
		if _, err := ldap.ParseDN(p.DN); err != nil || p.DN == "" {
			return fmt.Errorf("page %s: invalid dn %q", p.ID, p.DN)
		}
	}
	if len(p.Fields) == 0 {
		return fmt.Errorf("page %s: no fields", p.ID)
	}
	return validateFields("page "+p.ID, p.Fields)
}

// Applies reports whether the page applies to a server with these facts
// (currently "dbImpl": mdb|bdb).
func (p *Page) Applies(facts map[string]string) bool {
	for k, v := range p.When {
		if !strings.EqualFold(facts[k], v) {
			return false
		}
	}
	return true
}

// Field returns the field for attr.
func (p *Page) Field(attr string) *PageField {
	for i := range p.Fields {
		if strings.EqualFold(p.Fields[i].Attr, attr) {
			return &p.Fields[i]
		}
	}
	return nil
}

// ErrInvalidValue is returned when a value does not fit its field.
var ErrInvalidValue = errors.New("invalid value")

// Normalize validates the values of one field and returns what to store.
// An empty result means "remove the attribute" (the server's default).
func (f *PageField) Normalize(values []string, options []string) ([]string, error) {
	var out []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if f.Type != TypeList && len(out) > 1 {
		return nil, fmt.Errorf("%w: %s takes one value", ErrInvalidValue, f.Attr)
	}
	for _, v := range out {
		switch f.Type {
		case TypeInt, TypeDuration, TypeSize, TypeFlags:
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w: %s must be an integer", ErrInvalidValue, f.Attr)
			}
			if f.Min != nil && n < *f.Min || f.Max != nil && n > *f.Max {
				return nil, fmt.Errorf("%w: %s is out of range", ErrInvalidValue, f.Attr)
			}
		case TypeBool:
			if !strings.EqualFold(v, f.Bool[0]) && !strings.EqualFold(v, f.Bool[1]) {
				return nil, fmt.Errorf("%w: %s must be %s or %s", ErrInvalidValue, f.Attr, f.Bool[0], f.Bool[1])
			}
		case TypeEnum:
			allowed := append(append([]string{}, f.Values...), options...)
			ok := false
			for _, a := range allowed {
				if strings.EqualFold(a, v) {
					ok = true
				}
			}
			if !ok {
				return nil, fmt.Errorf("%w: %s: %q is not one of the allowed values", ErrInvalidValue, f.Attr, v)
			}
		case TypeDN:
			if _, err := ldap.ParseDN(v); err != nil {
				return nil, fmt.Errorf("%w: %s must be a DN", ErrInvalidValue, f.Attr)
			}
		}
	}
	if f.ReadOnly {
		return nil, fmt.Errorf("%w: %s is read-only", ErrInvalidValue, f.Attr)
	}
	return out, nil
}

// validateFields checks the fields of a page or collection and fills
// defaults (bool literals).
func validateFields(owner string, fields []PageField) error {
	attrs := map[string]bool{}
	for i := range fields {
		f := &fields[i]
		if f.Attr == "" || strings.ContainsAny(f.Attr, "{}") {
			return fmt.Errorf("%s: field %d: invalid attribute %q", owner, i, f.Attr)
		}
		if attrs[strings.ToLower(f.Attr)] {
			return fmt.Errorf("%s: duplicate field %s", owner, f.Attr)
		}
		attrs[strings.ToLower(f.Attr)] = true
		if !types[f.Type] {
			return fmt.Errorf("%s: field %s: unknown type %q", owner, f.Attr, f.Type)
		}
		if f.Type == TypeEnum && len(f.Values) == 0 && f.OptionsFrom == nil {
			return fmt.Errorf("%s: enum field %s has no values", owner, f.Attr)
		}
		if f.Type == TypeBool {
			if f.Bool == nil {
				f.Bool = []string{"on", "off"}
			}
			if len(f.Bool) != 2 {
				return fmt.Errorf("%s: field %s: bool needs [true, false] literals", owner, f.Attr)
			}
		}
		if f.Type == TypeFlags && len(f.Flags) == 0 {
			return fmt.Errorf("%s: flags field %s has no flags", owner, f.Attr)
		}
	}
	return nil
}
