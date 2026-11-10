package pages

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadEmbedded(t *testing.T) {
	ps, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*Page{}
	for _, p := range ps {
		byID[p.ID] = p
	}
	for _, id := range []string{"server-general", "server-limits", "db-global", "db-mdb", "log-access",
		"log-errors", "log-audit", "log-auditfail", "log-security", "security-binds", "ldapi", "password-policy"} {
		if byID[id] == nil {
			t.Errorf("page %s missing", id)
		}
	}
	// Fieldsets are expanded with the page's vars.
	if f := byID["log-errors"].Field("nsslapd-errorlog-maxlogsize"); f == nil || f.Unit != "MB" || f.Text != "log.maxlogsize" {
		t.Errorf("expanded field = %+v", f)
	}
	// Expansion must not leak between pages sharing a fieldset.
	if byID["log-access"].Field("nsslapd-errorlog-maxlogsize") != nil {
		t.Error("fieldset shared by reference")
	}
	if f := byID["server-general"].Field("nsslapd-readonly"); f.Bool[0] != "on" || f.Bool[1] != "off" {
		t.Errorf("bool default = %v", f.Bool)
	}
	// Sorted by section, then order.
	for i := 1; i < len(ps); i++ {
		a, b := ps[i-1], ps[i]
		if a.Section > b.Section || a.Section == b.Section && a.Order > b.Order {
			t.Errorf("order: %s before %s", a.ID, b.ID)
		}
	}
}

func TestLoadRejects(t *testing.T) {
	cases := map[string]string{
		"unknown key":  "pages:\n- {id: a, section: s, dn: cn=config, colour: red, fields: [{attr: x, type: int}]}",
		"bad type":     "pages:\n- {id: a, section: s, dn: cn=config, fields: [{attr: x, type: float}]}",
		"bad id":       "pages:\n- {id: A_b, section: s, dn: cn=config, fields: [{attr: x, type: int}]}",
		"bad dn":       "pages:\n- {id: a, section: s, dn: 'nonsense', fields: [{attr: x, type: int}]}",
		"enum no vals": "pages:\n- {id: a, section: s, dn: cn=config, fields: [{attr: x, type: enum}]}",
		"duplicate":    "pages:\n- {id: a, section: s, dn: cn=config, fields: [{attr: x, type: int}, {attr: X, type: int}]}",
		"unknown set":  "pages:\n- {id: a, section: s, dn: cn=config, use: [nope], fields: [{attr: x, type: int}]}",
		"unset var":    "pages:\n- {id: a, section: s, dn: cn=config, fields: [{attr: 'x-{log}', type: int}]}",
	}
	for name, src := range cases {
		_, err := load(fstest.MapFS{"defs/x.yaml": {Data: []byte(src)}})
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestNormalize(t *testing.T) {
	ptr := func(n int64) *int64 { return &n }
	intF := PageField{Attr: "n", Type: TypeInt, Min: ptr(0), Max: ptr(10)}
	if v, err := intF.Normalize([]string{" 5 "}, nil); err != nil || v[0] != "5" {
		t.Errorf("int: %v %v", v, err)
	}
	for _, bad := range []string{"11", "-1", "x"} {
		if _, err := intF.Normalize([]string{bad}, nil); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("int %q: %v", bad, err)
		}
	}
	if v, err := intF.Normalize([]string{"", " "}, nil); err != nil || len(v) != 0 {
		t.Errorf("empty means default: %v %v", v, err)
	}
	if _, err := intF.Normalize([]string{"1", "2"}, nil); err == nil {
		t.Error("two values for a single-valued field")
	}
	boolF := PageField{Attr: "b", Type: TypeBool, Bool: []string{"on", "off"}}
	if _, err := boolF.Normalize([]string{"ON"}, nil); err != nil {
		t.Error(err)
	}
	if _, err := boolF.Normalize([]string{"yes"}, nil); err == nil {
		t.Error("bool accepted yes")
	}
	enumF := PageField{Attr: "e", Type: TypeEnum, Values: []string{"a"}}
	if _, err := enumF.Normalize([]string{"SSHA"}, []string{"SSHA", "PBKDF2"}); err != nil {
		t.Error("server option rejected:", err)
	}
	if _, err := enumF.Normalize([]string{"z"}, nil); err == nil {
		t.Error("enum accepted z")
	}
	dnF := PageField{Attr: "d", Type: TypeDN}
	if _, err := dnF.Normalize([]string{"nonsense"}, nil); err == nil {
		t.Error("dn accepted nonsense")
	}
	listF := PageField{Attr: "l", Type: TypeList}
	if v, _ := listF.Normalize([]string{"PLAIN", "", "EXTERNAL"}, nil); strings.Join(v, ",") != "PLAIN,EXTERNAL" {
		t.Errorf("list = %v", v)
	}
	roF := PageField{Attr: "r", Type: TypeString, ReadOnly: true}
	if _, err := roF.Normalize([]string{"x"}, nil); err == nil {
		t.Error("read-only accepted")
	}
}

func TestSameValues(t *testing.T) {
	if !sameValues([]string{""}, nil) || !sameValues([]string{" a "}, []string{"a"}) || sameValues([]string{"a"}, []string{"b"}) {
		t.Error("sameValues")
	}
}

func TestApplies(t *testing.T) {
	p := &Page{When: map[string]string{"dbImpl": "mdb"}}
	if !p.Applies(map[string]string{"dbImpl": "MDB"}) || p.Applies(map[string]string{"dbImpl": "bdb"}) {
		t.Error("Applies")
	}
}
