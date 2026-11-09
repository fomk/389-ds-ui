package entries

import (
	"errors"
	"slices"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

func TestParseLDIFRejectsURLValues(t *testing.T) {
	for _, text := range []string{
		"dn: cn=x,dc=example\nobjectClass: top\njpegPhoto:< file:///etc/passwd\n",
		"dn: cn=x,dc=example\nchangetype: modify\nreplace: description\ndescription:<file:///etc/shadow\n-\n",
	} {
		if _, err := parseLDIF(text); !errors.Is(err, ErrInvalidLDIF) {
			t.Errorf("URL value must be rejected: %v", err)
		}
	}
	// "<" inside a normal value is fine.
	if _, err := parseLDIF("dn: cn=x,dc=example\nobjectClass: top\ncn: x\ndescription: a <b>\n"); err != nil {
		t.Errorf("plain value with '<': %v", err)
	}
}

func TestParseLDIFErrors(t *testing.T) {
	if _, err := parseLDIF("this is not ldif"); !errors.Is(err, ErrInvalidLDIF) {
		t.Errorf("garbage: %v", err)
	}
}

func TestDiffRaw(t *testing.T) {
	sch := testSchema(t)
	cur := ldap.NewEntry("cn=a,dc=x", map[string][]string{
		"objectClass":  {"top", "person"},
		"cn":           {"a"},
		"description":  {"old", "keep"},
		"userPassword": {"{SSHA}hash"},
		"jpegPhoto":    {"\xff\xd8"},
	})
	want := ldap.NewEntry("cn=a,dc=x", map[string][]string{
		"objectClass": {"person", "top"},
		"CN":          {"a"},
		"description": {"keep", "new"},
		"mail":        {"a@x"},
	})
	changes := diffRaw(cur, want, sch)
	var got []string
	for _, c := range changes {
		got = append(got, c.Op+":"+c.Attr)
	}
	slices.Sort(got)
	wantOps := []string{"delete:jpegPhoto", "replace:description", "replace:mail"}
	if !slices.Equal(got, wantOps) {
		t.Errorf("changes = %v, want %v (objectClass/cn unchanged, password untouched)", got, wantOps)
	}
	for _, c := range changes {
		if c.Op == "replace" && !c.Binary {
			t.Errorf("%s values must be sent base64-encoded", c.Attr)
		}
	}
}

func TestScopeOf(t *testing.T) {
	for in, want := range map[string]int{"base": ldap.ScopeBaseObject, "one": ldap.ScopeSingleLevel, "sub": ldap.ScopeWholeSubtree, "": ldap.ScopeWholeSubtree} {
		if got, err := scopeOf(in); err != nil || got != want {
			t.Errorf("scopeOf(%q) = %d, %v", in, got, err)
		}
	}
	if _, err := scopeOf("children"); err == nil {
		t.Error("unknown scope must fail")
	}
}

func TestValidateExport(t *testing.T) {
	if err := ValidateExport(ExportRequest{Base: "dc=x", Filter: "(cn=*"}); !errors.Is(err, ErrInvalidFilter) {
		t.Errorf("bad filter: %v", err)
	}
	if err := ValidateExport(ExportRequest{Base: "", Filter: "(cn=*)"}); !errors.Is(err, ErrInvalidDN) {
		t.Errorf("empty base: %v", err)
	}
	if err := ValidateExport(ExportRequest{Base: "dc=x", Filter: "", Scope: "one"}); err != nil {
		t.Errorf("valid request: %v", err)
	}
}
