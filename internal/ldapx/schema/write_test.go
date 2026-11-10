package schema

import (
	"errors"
	"testing"
)

func testSchema(t *testing.T) *Schema {
	t.Helper()
	s, errs := Parse(Raw{
		Syntaxes:      []string{"( 1.3.6.1.4.1.1466.115.121.1.15 DESC 'Directory String' )"},
		MatchingRules: []string{"( 2.5.13.2 NAME 'caseIgnoreMatch' SYNTAX 1.3.6.1.4.1.1466.115.121.1.15 )"},
		AttributeTypes: []string{
			"( 2.5.4.41 NAME 'name' EQUALITY caseIgnoreMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.15 )",
			"( 2.5.4.3 NAME 'cn' SUP name )",
		},
		ObjectClasses: []string{"( 2.5.6.0 NAME 'top' ABSTRACT MUST objectClass )"},
	})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	return s
}

func TestAttributeTypeDef(t *testing.T) {
	s := testSchema(t)
	d := AttributeTypeDef{OID: " 1.3.6.1.4.1.99999.1.1 ", Names: []string{"badgeNumber", ""}, Desc: "Bob's badge",
		Equality: "caseIgnoreMatch", Syntax: "1.3.6.1.4.1.1466.115.121.1.15", SingleValue: true}
	if err := d.Normalize(s); err != nil {
		t.Fatal(err)
	}
	want := `( 1.3.6.1.4.1.99999.1.1 NAME 'badgeNumber' DESC 'Bob\27s badge' EQUALITY caseIgnoreMatch SYNTAX 1.3.6.1.4.1.1466.115.121.1.15 SINGLE-VALUE X-ORIGIN 'user defined' )`
	if got := d.String(); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	// The generated text parses back to the same definition.
	back, errs := Parse(Raw{AttributeTypes: []string{d.String()}})
	if len(errs) > 0 || back.AttributeTypes[0].Desc != "Bob's badge" || !UserDefined(back.AttributeTypes[0].Origin) {
		t.Errorf("round trip: %+v %v", back.AttributeTypes[0], errs)
	}

	bad := []AttributeTypeDef{
		{OID: "1.2", Names: []string{"x"}},                               // neither syntax nor sup
		{OID: "abc", Names: []string{"x"}, Sup: "name"},                  // bad OID
		{OID: "x-oid", Names: []string{"1x"}, Sup: "name"},               // bad name
		{OID: "x-oid", Names: []string{"x"}, Sup: "nope"},                // unknown sup
		{OID: "x-oid", Names: []string{"x"}, Sup: "name", Equality: "z"}, // unknown rule
		{OID: "x-oid", Names: []string{"x"}, Syntax: "1.2.3"},            // unknown syntax
	}
	for i, b := range bad {
		if err := b.Normalize(s); !errors.Is(err, ErrInvalidDefinition) {
			t.Errorf("%d: %v", i, err)
		}
	}
}

func TestObjectClassDef(t *testing.T) {
	s := testSchema(t)
	d := ObjectClassDef{OID: "badge-oid", Names: []string{"badgeHolder"}, Sup: []string{"top"}, Kind: ClassAuxiliary,
		Must: []string{"cn"}, May: []string{"name", "cn"}}
	if err := d.Normalize(s); err != nil {
		t.Fatal(err)
	}
	want := `( badge-oid NAME 'badgeHolder' SUP top AUXILIARY MUST cn MAY ( name $ cn ) X-ORIGIN 'user defined' )`
	if got := d.String(); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if err := (&ObjectClassDef{OID: "a-oid", Names: []string{"a"}, May: []string{"nope"}}).Normalize(s); err == nil {
		t.Error("unknown attribute accepted")
	}
	if ParsedOID(d.String()) != "badge-oid" {
		t.Error("ParsedOID")
	}
}
