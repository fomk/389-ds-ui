package templates

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedTemplatesAreValid(t *testing.T) {
	list, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, tpl := range list {
		ids = append(ids, tpl.ID)
	}
	if got := strings.Join(ids, ","); got != "user,posix-user,group,posix-group,ou,custom" {
		t.Errorf("templates = %s", got)
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]string{
		"bad id":        "id: Bad_ID\nobjectClasses: [top]\nrdn: cn\nfields: [{name: cn}]\n",
		"no classes":    "id: x\nrdn: cn\nfields: [{name: cn}]\n",
		"rdn not field": "id: x\nobjectClasses: [top]\nrdn: uid\nfields: [{name: cn}]\n",
		"unknown key":   "id: x\nobjectClasses: [top]\nrdn: cn\nfields: [{name: cn}]\ncolour: red\n",
	}
	for name, doc := range cases {
		_, err := load(fstest.MapFS{"defs/a.yaml": {Data: []byte(doc)}})
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	_, err := load(fstest.MapFS{
		"defs/a.yaml": {Data: []byte("id: x\nobjectClasses: [top]\nrdn: cn\nfields: [{name: cn}]\n")},
		"defs/b.yaml": {Data: []byte("id: x\nobjectClasses: [top]\nrdn: cn\nfields: [{name: cn}]\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("duplicate ids: %v", err)
	}
}
