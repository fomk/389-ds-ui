package schema

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// UserOrigin is the X-ORIGIN 389-ds gives definitions added over LDAP; they
// are stored in 99user.ldif and are the only ones ds-ui changes.
const UserOrigin = "user defined"

// ErrInvalidDefinition is returned for a definition that cannot be written.
var ErrInvalidDefinition = errors.New("invalid schema definition")

var (
	numericOID = regexp.MustCompile(`^[0-2](\.(0|[1-9][0-9]*))+$`)
	// 389-ds also accepts descriptive OIDs such as "myattr-oid".
	textOID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*-oid$`)
	keyName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)
)

// UserDefined reports whether a definition was added by an administrator.
func UserDefined(origin []string) bool {
	for _, o := range origin {
		if strings.EqualFold(o, UserOrigin) {
			return true
		}
	}
	return false
}

// AttributeTypeDef is a new or changed attribute type.
type AttributeTypeDef struct {
	OID         string   `json:"oid" doc:"Numeric OID, or name-oid"`
	Names       []string `json:"names" minItems:"1"`
	Desc        string   `json:"desc,omitempty"`
	Sup         string   `json:"sup,omitempty"`
	Equality    string   `json:"equality,omitempty"`
	Ordering    string   `json:"ordering,omitempty"`
	Substr      string   `json:"substr,omitempty"`
	Syntax      string   `json:"syntax,omitempty" doc:"Syntax OID; required unless sup is set"`
	SingleValue bool     `json:"singleValue,omitempty"`
}

// ObjectClassDef is a new or changed object class.
type ObjectClassDef struct {
	OID   string    `json:"oid"`
	Names []string  `json:"names" minItems:"1"`
	Desc  string    `json:"desc,omitempty"`
	Sup   []string  `json:"sup,omitempty"`
	Kind  ClassKind `json:"kind" enum:"structural,auxiliary,abstract"`
	Must  []string  `json:"must"`
	May   []string  `json:"may"`
}

func qd(s string) string {
	return "'" + strings.NewReplacer(`\`, `\5C`, `'`, `\27`).Replace(s) + "'"
}

func names(n []string) string {
	if len(n) == 1 {
		return qd(n[0])
	}
	q := make([]string, len(n))
	for i, s := range n {
		q[i] = qd(s)
	}
	return "( " + strings.Join(q, " ") + " )"
}

func oids(list []string) string {
	if len(list) == 1 {
		return list[0]
	}
	return "( " + strings.Join(list, " $ ") + " )"
}

func validOID(oid string) error {
	if !numericOID.MatchString(oid) && !textOID.MatchString(oid) {
		return fmt.Errorf("%w: OID %q (use a numeric OID or name-oid)", ErrInvalidDefinition, oid)
	}
	return nil
}

func validNames(n []string) error {
	if len(n) == 0 {
		return fmt.Errorf("%w: at least one name is needed", ErrInvalidDefinition)
	}
	for _, s := range n {
		if !keyName.MatchString(s) {
			return fmt.Errorf("%w: name %q (letters, digits and hyphens, starting with a letter)", ErrInvalidDefinition, s)
		}
	}
	return nil
}

func clean(list []string) []string {
	out := []string{}
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Normalize trims the definition and checks it against the schema it is
// added to: referenced attribute types, classes, rules and syntaxes must
// exist.
func (d *AttributeTypeDef) Normalize(s *Schema) error {
	d.OID, d.Desc, d.Sup = strings.TrimSpace(d.OID), strings.TrimSpace(d.Desc), strings.TrimSpace(d.Sup)
	d.Equality, d.Ordering, d.Substr = strings.TrimSpace(d.Equality), strings.TrimSpace(d.Ordering), strings.TrimSpace(d.Substr)
	d.Syntax, d.Names = strings.TrimSpace(d.Syntax), clean(d.Names)
	if err := validOID(d.OID); err != nil {
		return err
	}
	if err := validNames(d.Names); err != nil {
		return err
	}
	if d.Sup == "" && d.Syntax == "" {
		return fmt.Errorf("%w: a syntax or a superior type is needed", ErrInvalidDefinition)
	}
	if d.Sup != "" && s.Attr(d.Sup) == nil {
		return fmt.Errorf("%w: unknown superior type %q", ErrInvalidDefinition, d.Sup)
	}
	if d.Syntax != "" && !s.hasSyntax(d.Syntax) {
		return fmt.Errorf("%w: unknown syntax %q", ErrInvalidDefinition, d.Syntax)
	}
	for _, r := range []string{d.Equality, d.Ordering, d.Substr} {
		if r != "" && !s.hasRule(r) {
			return fmt.Errorf("%w: unknown matching rule %q", ErrInvalidDefinition, r)
		}
	}
	return nil
}

// String is the RFC 4512 form with X-ORIGIN 'user defined'.
func (d *AttributeTypeDef) String() string {
	var b strings.Builder
	b.WriteString("( " + d.OID + " NAME " + names(d.Names))
	if d.Desc != "" {
		b.WriteString(" DESC " + qd(d.Desc))
	}
	if d.Sup != "" {
		b.WriteString(" SUP " + d.Sup)
	}
	if d.Equality != "" {
		b.WriteString(" EQUALITY " + d.Equality)
	}
	if d.Ordering != "" {
		b.WriteString(" ORDERING " + d.Ordering)
	}
	if d.Substr != "" {
		b.WriteString(" SUBSTR " + d.Substr)
	}
	if d.Syntax != "" {
		b.WriteString(" SYNTAX " + d.Syntax)
	}
	if d.SingleValue {
		b.WriteString(" SINGLE-VALUE")
	}
	b.WriteString(" X-ORIGIN " + qd(UserOrigin) + " )")
	return b.String()
}

// Normalize trims the definition and checks references against s.
func (d *ObjectClassDef) Normalize(s *Schema) error {
	d.OID, d.Desc = strings.TrimSpace(d.OID), strings.TrimSpace(d.Desc)
	d.Names, d.Sup, d.Must, d.May = clean(d.Names), clean(d.Sup), clean(d.Must), clean(d.May)
	if d.Kind == "" {
		d.Kind = ClassStructural
	}
	if d.Kind != ClassStructural && d.Kind != ClassAuxiliary && d.Kind != ClassAbstract {
		return fmt.Errorf("%w: kind %q", ErrInvalidDefinition, d.Kind)
	}
	if err := validOID(d.OID); err != nil {
		return err
	}
	if err := validNames(d.Names); err != nil {
		return err
	}
	for _, c := range d.Sup {
		if s.Class(c) == nil {
			return fmt.Errorf("%w: unknown superior class %q", ErrInvalidDefinition, c)
		}
	}
	for _, a := range append(append([]string{}, d.Must...), d.May...) {
		if s.Attr(a) == nil {
			return fmt.Errorf("%w: unknown attribute type %q", ErrInvalidDefinition, a)
		}
	}
	return nil
}

// String is the RFC 4512 form with X-ORIGIN 'user defined'.
func (d *ObjectClassDef) String() string {
	var b strings.Builder
	b.WriteString("( " + d.OID + " NAME " + names(d.Names))
	if d.Desc != "" {
		b.WriteString(" DESC " + qd(d.Desc))
	}
	if len(d.Sup) > 0 {
		b.WriteString(" SUP " + oids(d.Sup))
	}
	b.WriteString(" " + strings.ToUpper(string(d.Kind)))
	if len(d.Must) > 0 {
		b.WriteString(" MUST " + oids(d.Must))
	}
	if len(d.May) > 0 {
		b.WriteString(" MAY " + oids(d.May))
	}
	b.WriteString(" X-ORIGIN " + qd(UserOrigin) + " )")
	return b.String()
}

func (s *Schema) hasSyntax(oid string) bool {
	for _, x := range s.Syntaxes {
		if x.OID == oid {
			return true
		}
	}
	return false
}

func (s *Schema) hasRule(r string) bool {
	for _, m := range s.MatchingRules {
		if strings.EqualFold(m.OID, r) {
			return true
		}
		for _, n := range m.Names {
			if strings.EqualFold(n, r) {
				return true
			}
		}
	}
	return false
}

// ParsedOID returns the OID of a raw definition, or "" if it does not parse.
func ParsedOID(raw string) string {
	d, err := parseDefinition(raw)
	if err != nil {
		return ""
	}
	return d.oid
}
