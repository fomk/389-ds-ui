package entries

import (
	"encoding/base64"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
)

func testSchema(t *testing.T) *schema.Schema {
	t.Helper()
	s, errs := schema.Parse(schema.Raw{AttributeTypes: []string{
		`( 2.5.4.3 NAME 'cn' SYNTAX 1.3.6.1.4.1.1466.115.121.1.15 )`,
		`( 2.5.4.0 NAME 'objectClass' SYNTAX 1.3.6.1.4.1.1466.115.121.1.38 )`,
		`( 2.5.4.35 NAME 'userPassword' SYNTAX 1.3.6.1.4.1.1466.115.121.1.40 )`,
		`( 2.5.4.36 NAME 'userCertificate' SYNTAX 1.3.6.1.4.1.1466.115.121.1.40 )`,
		`( 0.9.2342.19200300.100.1.60 NAME 'jpegPhoto' SYNTAX 1.3.6.1.4.1.1466.115.121.1.28 )`,
		`( 2.5.18.1 NAME 'createTimestamp' SYNTAX 1.3.6.1.4.1.1466.115.121.1.24 USAGE directoryOperation )`,
		`( 1.3.6.1.4.1.453.16.2.103 NAME 'numSubordinates' SYNTAX 1.3.6.1.4.1.1466.115.121.1.27 USAGE directoryOperation )`,
		`( 2.5.4.13 NAME 'description' SYNTAX 1.3.6.1.4.1.1466.115.121.1.15 )`,
	}})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	return s
}

func rawEntry(dn string, attrs map[string][][]byte) *ldap.Entry {
	e := &ldap.Entry{DN: dn}
	for name, vals := range attrs {
		a := &ldap.EntryAttribute{Name: name, ByteValues: vals}
		for _, v := range vals {
			a.Values = append(a.Values, string(v))
		}
		e.Attributes = append(e.Attributes, a)
	}
	return e
}

func find(e *Entry, name string) *Attribute {
	for i := range e.Attributes {
		if e.Attributes[i].Name == name {
			return &e.Attributes[i]
		}
	}
	return nil
}

func TestConvert(t *testing.T) {
	sch := testSchema(t)
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00}
	e := convert(rawEntry("uid=jdoe,ou=People,dc=example,dc=com", map[string][][]byte{
		"objectClass":     {[]byte("top"), []byte("person")},
		"cn":              {[]byte("John Doe")},
		"cn;lang-ru":      {[]byte("Джон")},
		"userPassword":    {[]byte("{PBKDF2-SHA512}10000$abc$def"), []byte("plain")},
		"jpegPhoto":       {jpeg},
		"userCertificate": {[]byte{0x30, 0x82, 0x01}},
		"description":     {[]byte("printable octet\nwith newline")},
		"createTimestamp": {[]byte("20261109122001Z")},
		"numSubordinates": {[]byte("3")},
	}), sch)

	if e.RDN != "uid=jdoe" || e.Parent != "ou=People,dc=example,dc=com" {
		t.Errorf("rdn/parent = %q / %q", e.RDN, e.Parent)
	}
	if len(e.ObjectClasses) != 2 || e.NumSubordinates != 3 || !e.HasChildren {
		t.Errorf("entry = %+v", e)
	}
	if e.Attributes[0].Name != "objectClass" || !e.Attributes[len(e.Attributes)-1].Operational {
		t.Errorf("order: objectClass first, operational last: %+v", e.Attributes)
	}

	pw := find(e, "userPassword")
	if !pw.Redacted || pw.Values[0] != "{PBKDF2-SHA512}" || pw.Values[1] != "{CLEAR}" {
		t.Errorf("password = %+v", pw)
	}
	if p := find(e, "jpegPhoto"); !p.Binary || p.Values[0] != base64.StdEncoding.EncodeToString(jpeg) {
		t.Errorf("jpegPhoto = %+v", p)
	}
	if c := find(e, "userCertificate"); !c.Binary {
		t.Errorf("non-UTF-8 octet string must be binary: %+v", c)
	}
	if ru := find(e, "cn;lang-ru"); ru.Binary || ru.Values[0] != "Джон" {
		t.Errorf("language option = %+v", ru)
	}
	if d := find(e, "description"); d.Binary {
		t.Errorf("text with newline must stay text: %+v", d)
	}
	if ct := find(e, "createTimestamp"); !ct.Operational {
		t.Errorf("createTimestamp = %+v", ct)
	}
}

func TestETag(t *testing.T) {
	sch := testSchema(t)
	base := map[string][][]byte{"cn": {[]byte("a"), []byte("b")}, "objectClass": {[]byte("top")}}
	a := convert(rawEntry("cn=a", base), sch).ETag

	reordered := map[string][][]byte{"objectClass": {[]byte("top")}, "cn": {[]byte("b"), []byte("a")},
		"createTimestamp": {[]byte("20990101000000Z")}}
	if b := convert(rawEntry("cn=a", reordered), sch).ETag; a != b {
		t.Errorf("etag must ignore value order and operational attributes: %s != %s", a, b)
	}
	changed := map[string][][]byte{"cn": {[]byte("a")}, "objectClass": {[]byte("top")}}
	if c := convert(rawEntry("cn=a", changed), sch).ETag; a == c {
		t.Error("etag must change when a value changes")
	}
}

func TestChildCache(t *testing.T) {
	c := newChildCache(1<<62, 2)
	c.put("a", []Summary{{DN: "a"}})
	c.put("b", []Summary{{DN: "b"}})
	c.put("c", []Summary{{DN: "c"}})
	if _, ok := c.get("a"); ok {
		t.Error("oldest entry must be evicted")
	}
	if _, ok := c.get("c"); !ok {
		t.Error("newest entry missing")
	}
	c.drop("c")
	if _, ok := c.get("c"); ok {
		t.Error("dropped entry still cached")
	}
}
