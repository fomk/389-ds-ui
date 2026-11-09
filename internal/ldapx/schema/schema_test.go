package schema

import (
	"slices"
	"testing"
)

// Definitions copied from a 389-ds 3.1.2 cn=schema (formatting kept as served,
// including the double spaces 389 emits).
var raw389 = Raw{
	AttributeTypes: []string{
		`( 2.5.4.41 NAME 'name'  EQUALITY caseIgnoreMatch SUBSTR caseIgnoreSubstringsMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.15 X-ORIGIN 'RFC 4519' )`,
		`( 2.5.4.3 NAME ( 'cn' 'commonName' )  SUP name EQUALITY caseIgnoreMatch SUBSTR caseIgnoreSubstringsMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.15 X-ORIGIN 'RFC 4519' )`,
		`( 2.5.4.4 NAME ( 'sn' 'surName' )  SUP name X-ORIGIN 'RFC 4519' )`,
		`( 2.5.4.49 NAME 'distinguishedName'  EQUALITY distinguishedNameMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.12 X-ORIGIN 'RFC 4519' )`,
		`( 2.5.4.31 NAME 'member'  SUP distinguishedName EQUALITY distinguishedNameMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.12 X-ORIGIN 'RFC 4519' )`,
		`( 2.5.4.35 NAME 'userPassword'  EQUALITY octetStringMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.40 X-ORIGIN 'RFC 4519' )`,
		`( 2.5.4.36 NAME 'userCertificate' DESC 'X.509 user certificate' EQUALITY octetStringMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.40 X-ORIGIN 'RFC 4523' )`,
		`( 0.9.2342.19200300.100.1.60 NAME 'jpegPhoto' DESC 'a JPEG image' SYNTAX 1.3.6.1.4.1.1466.115.121.1.28 X-ORIGIN 'RFC 2798' )`,
		`( 1.3.6.1.1.1.1.0 NAME 'uidNumber' DESC 'An integer uniquely identifying a user in an administrative domain' EQUALITY integerMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.27 SINGLE-VALUE )`,
		`( 2.5.18.1 NAME 'createTimestamp'  EQUALITY generalizedTimeMatch ORDERING generalizedTimeOrderingMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.24 SINGLE-VALUE NO-USER-MODIFICATION USAGE directoryOperation X-ORIGIN 'RFC 4512' )`,
		`( 0.9.2342.19200300.100.1.3 NAME ( 'mail' 'rfc822mailbox' )  EQUALITY caseIgnoreIA5Match SUBSTR caseIgnoreIA5SubstringsMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.26 X-ORIGIN ( 'RFC 4524' 'user defined' ) )`,
		`( 0.9.2342.19200300.100.1.1 NAME ( 'uid' 'userid' )  EQUALITY caseIgnoreMatch SUBSTR caseIgnoreSubstringsMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.15{256} X-ORIGIN 'RFC 4519' )`,
		`( 2.5.4.0 NAME 'objectClass'  EQUALITY objectIdentifierMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.38 X-ORIGIN 'RFC 4512' )`,
		`( 2.5.4.13 NAME 'description'  EQUALITY caseIgnoreMatch SUBSTR caseIgnoreSubstringsMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.15 X-ORIGIN 'RFC 4519' )`,
		`( 1.2.3.4 NAME 'quoted' DESC 'It\27s a \5Ctest' SYNTAX 1.3.6.1.4.1.1466.115.121.1.15 )`,
	},
	ObjectClasses: []string{
		`( 2.5.6.0 NAME 'top' ABSTRACT MUST objectClass X-ORIGIN 'RFC 4512' )`,
		`( 2.5.6.6 NAME 'person' SUP top STRUCTURAL MUST ( sn $ cn ) MAY ( userPassword $ telephoneNumber $ seeAlso $ description ) X-ORIGIN 'RFC 4519' )`,
		`( 2.5.6.7 NAME 'organizationalPerson' SUP person STRUCTURAL MAY ( title $ ou ) X-ORIGIN 'RFC 4519' )`,
		`( 2.16.840.1.113730.3.2.2 NAME 'inetOrgPerson' SUP organizationalPerson STRUCTURAL MAY ( mail $ uid $ jpegPhoto $ userCertificate ) X-ORIGIN 'RFC 2798' )`,
		`( 1.3.6.1.1.1.2.0 NAME 'posixAccount' DESC 'Abstraction of an account with POSIX attributes' SUP top AUXILIARY MUST ( cn $ uid $ uidNumber $ gidNumber $ homeDirectory ) MAY ( userPassword $ loginShell $ gecos $ description ) )`,
		`( 2.5.6.9 NAME 'groupOfNames' SUP top STRUCTURAL MUST cn MAY ( member $ businessCategory $ seeAlso $ owner $ ou $ o $ description ) X-ORIGIN 'RFC 4519' )`,
		`( 1.3.6.1.4.1.1466.101.120.111 NAME 'extensibleObject' DESC 'RFC 4512: extensible object' SUP top AUXILIARY )`,
	},
	Syntaxes: []string{
		`( 1.3.6.1.4.1.1466.115.121.1.5 DESC 'Binary' )`,
		`( 1.3.6.1.4.1.1466.115.121.1.15 DESC 'Directory String' )`,
	},
}

func mustParse(t *testing.T) *Schema {
	t.Helper()
	s, errs := Parse(raw389)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return s
}

func TestParseAttributeTypes(t *testing.T) {
	s := mustParse(t)

	cn := s.Attr("commonName")
	if cn == nil || cn.Name() != "cn" || cn.OID != "2.5.4.3" || cn.Sup != "name" {
		t.Fatalf("cn = %+v", cn)
	}
	if s.Attr("CN") != cn || s.Attr("2.5.4.3") != cn || s.Attr("cn;lang-ru") != cn {
		t.Error("case-insensitive / OID / option lookup failed")
	}

	if sn := s.Attr("sn"); sn.Syntax != "1.3.6.1.4.1.1466.115.121.1.15" || sn.Equality != "caseIgnoreMatch" || sn.Kind != KindString {
		t.Errorf("sn should inherit syntax and equality from name: %+v", sn)
	}
	if uid := s.Attr("uid"); uid.MaxLen != 256 || uid.Syntax != "1.3.6.1.4.1.1466.115.121.1.15" {
		t.Errorf("uid syntax length bound: %+v", uid)
	}
	if mail := s.Attr("mail"); !slices.Equal(mail.Origin, []string{"RFC 4524", "user defined"}) {
		t.Errorf("mail origin = %v", mail.Origin)
	}
	ct := s.Attr("createTimestamp")
	if !ct.SingleValue || !ct.NoUserModification || !ct.Operational() || ct.Kind != KindTime {
		t.Errorf("createTimestamp = %+v", ct)
	}
	if q := s.Attr("quoted"); q.Desc != `It's a \test` {
		t.Errorf("qdstring escapes: %q", q.Desc)
	}

	kinds := map[string]Kind{
		"member": KindDN, "uidNumber": KindInteger, "jpegPhoto": KindBinary,
		"userCertificate": KindOctet, "userPassword": KindPassword, "description": KindString,
	}
	for name, want := range kinds {
		if got := s.Attr(name).Kind; got != want {
			t.Errorf("%s kind = %s, want %s", name, got, want)
		}
	}
}

func TestParseObjectClasses(t *testing.T) {
	s := mustParse(t)
	if top := s.Class("top"); top.Kind != ClassAbstract || !slices.Equal(top.Must, []string{"objectClass"}) {
		t.Errorf("top = %+v", top)
	}
	if pa := s.Class("posixaccount"); pa.Kind != ClassAuxiliary || len(pa.Must) != 5 || pa.Desc == "" {
		t.Errorf("posixAccount = %+v", pa)
	}
	chain := s.Superclasses("inetOrgPerson")
	var names []string
	for _, c := range chain {
		names = append(names, c.Name())
	}
	if !slices.Equal(names, []string{"inetOrgPerson", "organizationalPerson", "person", "top"}) {
		t.Errorf("superclasses = %v", names)
	}
}

func TestAllowed(t *testing.T) {
	s := mustParse(t)
	must, may := s.Allowed([]string{"top", "inetOrgPerson", "posixAccount"})
	wantMust := []string{"cn", "gidNumber", "homeDirectory", "objectClass", "sn", "uid", "uidNumber"}
	if !slices.Equal(must, wantMust) {
		t.Errorf("must = %v, want %v", must, wantMust)
	}
	for _, a := range []string{"mail", "jpegPhoto", "userPassword", "title", "loginShell", "description"} {
		if !slices.Contains(may, a) {
			t.Errorf("may lacks %s: %v", a, may)
		}
	}
	for _, a := range wantMust {
		if slices.Contains(may, a) {
			t.Errorf("may must not repeat MUST attribute %s", a)
		}
	}
	if s.AllowsAny([]string{"person"}) || !s.AllowsAny([]string{"person", "extensibleObject"}) {
		t.Error("AllowsAny")
	}
	// Unknown classes are ignored rather than failing.
	if must, _ := s.Allowed([]string{"noSuchClass"}); len(must) != 0 {
		t.Errorf("unknown class must = %v", must)
	}
}

func TestParseErrorsAreCollected(t *testing.T) {
	s, errs := Parse(Raw{AttributeTypes: []string{
		`( 1.1 NAME 'ok' SYNTAX 1.3.6.1.4.1.1466.115.121.1.15 )`,
		`( 1.2 NAME 'broken DESC 'x' )`,
		`no parens at all`,
	}})
	if len(errs) != 2 {
		t.Errorf("errs = %v", errs)
	}
	if s.Attr("ok") == nil {
		t.Error("valid definition must survive invalid neighbours")
	}
}

func TestSupCycleDoesNotHang(t *testing.T) {
	s, _ := Parse(Raw{AttributeTypes: []string{
		`( 1.1 NAME 'a' SUP b )`,
		`( 1.2 NAME 'b' SUP a )`,
	}, ObjectClasses: []string{
		`( 2.1 NAME 'x' SUP y MUST a )`,
		`( 2.2 NAME 'y' SUP x MUST b )`,
	}})
	if must, _ := s.Allowed([]string{"x"}); len(must) != 2 {
		t.Errorf("must = %v", must)
	}
}
