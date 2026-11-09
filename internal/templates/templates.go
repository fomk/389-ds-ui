// Package templates provides entry templates for the LDAP editor's
// "create entry" form. Definitions are YAML files embedded from defs/.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

//go:embed defs/*.yaml
var defs embed.FS

// Field is an attribute pre-filled in the form.
type Field struct {
	Name    string `yaml:"name" json:"name"`
	Default string `yaml:"default,omitempty" json:"default,omitempty" doc:"May reference other fields as {attr}; {next:attr} is the next free number"`
}

// Template describes a kind of entry to create.
type Template struct {
	ID            string   `yaml:"id" json:"id" doc:"Stable identifier; UI texts live under templates.<id> in the locale files"`
	Icon          string   `yaml:"icon" json:"icon" enum:"user,group,folder,entry"`
	Custom        bool     `yaml:"custom,omitempty" json:"custom,omitempty" doc:"The user chooses object classes and the RDN attribute"`
	ObjectClasses []string `yaml:"objectClasses" json:"objectClasses"`
	RDN           string   `yaml:"rdn" json:"rdn" doc:"Naming attribute"`
	Password      bool     `yaml:"password,omitempty" json:"password,omitempty" doc:"Ask for an initial userPassword"`
	Fields        []Field  `yaml:"fields" json:"fields"`
}

var idRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Load parses and validates the embedded templates, ordered by file name.
func Load() ([]Template, error) {
	return load(defs)
}

func load(fsys fs.FS) ([]Template, error) {
	files, err := fs.Glob(fsys, "defs/*.yaml")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	seen := map[string]bool{}
	out := make([]Template, 0, len(files))
	for _, f := range files {
		b, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var t Template
		dec := yaml.NewDecoder(strings.NewReader(string(b)))
		dec.KnownFields(true)
		if err := dec.Decode(&t); err != nil {
			return nil, fmt.Errorf("%s: %w", path.Base(f), err)
		}
		if err := t.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path.Base(f), err)
		}
		if seen[t.ID] {
			return nil, fmt.Errorf("%s: duplicate template id %q", path.Base(f), t.ID)
		}
		seen[t.ID] = true
		if t.Fields == nil {
			t.Fields = []Field{}
		}
		out = append(out, t)
	}
	return out, nil
}

func (t *Template) validate() error {
	switch {
	case !idRE.MatchString(t.ID):
		return fmt.Errorf("invalid id %q", t.ID)
	case len(t.ObjectClasses) == 0:
		return fmt.Errorf("template %s: objectClasses is empty", t.ID)
	case t.RDN == "":
		return fmt.Errorf("template %s: rdn is empty", t.ID)
	}
	if !t.Custom && !slicesContainsFold(t.Fields, t.RDN) {
		return fmt.Errorf("template %s: rdn attribute %s is not among the fields", t.ID, t.RDN)
	}
	return nil
}

func slicesContainsFold(fields []Field, name string) bool {
	for _, f := range fields {
		if strings.EqualFold(f.Name, name) {
			return true
		}
	}
	return false
}
