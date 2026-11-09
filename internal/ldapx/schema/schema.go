package schema

import (
	"slices"
	"strings"
)

// Kind tells the UI which widget and encoding to use for an attribute.
type Kind string

const (
	KindString   Kind = "string"
	KindInteger  Kind = "integer"
	KindBoolean  Kind = "boolean"
	KindDN       Kind = "dn"
	KindTime     Kind = "time"     // GeneralizedTime / UTCTime
	KindBinary   Kind = "binary"   // always transferred as base64
	KindOctet    Kind = "octet"    // Octet String: text if the value is printable UTF-8, otherwise binary
	KindPassword Kind = "password" // write-mostly; values are hashes
)

// Well-known syntax OIDs (RFC 4517 and others).
const (
	syntaxPrefix          = "1.3.6.1.4.1.1466.115.121.1."
	SyntaxAudio           = syntaxPrefix + "4"
	SyntaxBinary          = syntaxPrefix + "5"
	SyntaxBoolean         = syntaxPrefix + "7"
	SyntaxCertificate     = syntaxPrefix + "8"
	SyntaxCertList        = syntaxPrefix + "9"
	SyntaxCertPair        = syntaxPrefix + "10"
	SyntaxDN              = syntaxPrefix + "12"
	SyntaxFax             = syntaxPrefix + "23"
	SyntaxGeneralizedTime = syntaxPrefix + "24"
	SyntaxInteger         = syntaxPrefix + "27"
	SyntaxJPEG            = syntaxPrefix + "28"
	SyntaxNameAndUID      = syntaxPrefix + "34"
	SyntaxOctetString     = syntaxPrefix + "40"
	SyntaxSupportedAlgo   = syntaxPrefix + "49"
	SyntaxUTCTime         = syntaxPrefix + "53"
)

// passwordAttrs hold secrets or hashes; the UI never shows them in clear.
var passwordAttrs = map[string]bool{
	"userpassword": true, "authpassword": true, "nsslapd-rootpw": true,
	"nsds5replicacredentials": true, "nsmultiplexorcredentials": true,
}

func kindForSyntax(oid string) Kind {
	switch oid {
	case SyntaxDN, SyntaxNameAndUID:
		return KindDN
	case SyntaxGeneralizedTime, SyntaxUTCTime:
		return KindTime
	case SyntaxInteger:
		return KindInteger
	case SyntaxBoolean:
		return KindBoolean
	case SyntaxBinary, SyntaxCertificate, SyntaxCertList, SyntaxCertPair, SyntaxJPEG,
		SyntaxAudio, SyntaxFax, SyntaxSupportedAlgo:
		return KindBinary
	case SyntaxOctetString:
		return KindOctet
	default:
		return KindString
	}
}

// AttributeType is an attribute definition with inherited properties resolved.
type AttributeType struct {
	OID                string   `json:"oid"`
	Names              []string `json:"names"`
	Desc               string   `json:"desc,omitempty"`
	Sup                string   `json:"sup,omitempty"`
	Equality           string   `json:"equality,omitempty"`
	Ordering           string   `json:"ordering,omitempty"`
	Substr             string   `json:"substr,omitempty"`
	Syntax             string   `json:"syntax,omitempty" doc:"Syntax OID, inherited from SUP if not set"`
	MaxLen             int      `json:"maxLen,omitempty"`
	SingleValue        bool     `json:"singleValue,omitempty"`
	Collective         bool     `json:"collective,omitempty"`
	NoUserModification bool     `json:"noUserModification,omitempty"`
	Obsolete           bool     `json:"obsolete,omitempty"`
	Usage              string   `json:"usage,omitempty" enum:"userApplications,directoryOperation,distributedOperation,dSAOperation"`
	Origin             []string `json:"origin,omitempty"`
	Kind               Kind     `json:"kind" enum:"string,integer,boolean,dn,time,binary,octet,password"`
}

// Name is the primary (first) name, or the OID if the type has no name.
func (a *AttributeType) Name() string {
	if len(a.Names) > 0 {
		return a.Names[0]
	}
	return a.OID
}

// Operational reports whether the attribute is maintained by the server.
func (a *AttributeType) Operational() bool {
	return a.Usage != "" && a.Usage != "userApplications"
}

// ClassKind is the kind of an object class.
type ClassKind string

const (
	ClassStructural ClassKind = "structural"
	ClassAuxiliary  ClassKind = "auxiliary"
	ClassAbstract   ClassKind = "abstract"
)

// ObjectClass is an object class definition (own MUST/MAY only).
type ObjectClass struct {
	OID      string    `json:"oid"`
	Names    []string  `json:"names"`
	Desc     string    `json:"desc,omitempty"`
	Sup      []string  `json:"sup,omitempty"`
	Kind     ClassKind `json:"kind" enum:"structural,auxiliary,abstract"`
	Must     []string  `json:"must"`
	May      []string  `json:"may"`
	Obsolete bool      `json:"obsolete,omitempty"`
	Origin   []string  `json:"origin,omitempty"`
}

// Name is the primary name of the class.
func (o *ObjectClass) Name() string {
	if len(o.Names) > 0 {
		return o.Names[0]
	}
	return o.OID
}

// Syntax is an LDAP syntax description.
type Syntax struct {
	OID  string `json:"oid"`
	Desc string `json:"desc,omitempty"`
}

// Schema is a parsed subschema with lookup indexes.
type Schema struct {
	AttributeTypes []*AttributeType `json:"attributeTypes"`
	ObjectClasses  []*ObjectClass   `json:"objectClasses"`
	Syntaxes       []Syntax         `json:"syntaxes"`

	attrs   map[string]*AttributeType
	classes map[string]*ObjectClass
}

// Raw holds the unparsed values of a subschema entry.
type Raw struct {
	AttributeTypes []string
	ObjectClasses  []string
	Syntaxes       []string
}

// Parse builds a Schema. Definitions that fail to parse are skipped and
// returned as errors so that one bad custom definition does not break the UI.
func Parse(raw Raw) (*Schema, []error) {
	s := &Schema{
		attrs:          map[string]*AttributeType{},
		classes:        map[string]*ObjectClass{},
		AttributeTypes: []*AttributeType{},
		ObjectClasses:  []*ObjectClass{},
		Syntaxes:       []Syntax{},
	}
	var errs []error

	for _, v := range raw.Syntaxes {
		d, err := parseDefinition(v)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		s.Syntaxes = append(s.Syntaxes, Syntax{OID: d.oid, Desc: d.first("DESC")})
	}

	for _, v := range raw.AttributeTypes {
		d, err := parseDefinition(v)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		syntax, maxLen := splitSyntax(d.first("SYNTAX"))
		a := &AttributeType{
			OID:                d.oid,
			Names:              nonNil(d.fields["NAME"]),
			Desc:               d.first("DESC"),
			Sup:                d.first("SUP"),
			Equality:           d.first("EQUALITY"),
			Ordering:           d.first("ORDERING"),
			Substr:             d.first("SUBSTR"),
			Syntax:             syntax,
			MaxLen:             maxLen,
			SingleValue:        d.has("SINGLE-VALUE"),
			Collective:         d.has("COLLECTIVE"),
			NoUserModification: d.has("NO-USER-MODIFICATION"),
			Obsolete:           d.has("OBSOLETE"),
			Usage:              d.first("USAGE"),
			Origin:             d.fields["X-ORIGIN"],
		}
		s.AttributeTypes = append(s.AttributeTypes, a)
		s.attrs[strings.ToLower(a.OID)] = a
		for _, n := range a.Names {
			s.attrs[strings.ToLower(n)] = a
		}
	}

	for _, v := range raw.ObjectClasses {
		d, err := parseDefinition(v)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		o := &ObjectClass{
			OID:      d.oid,
			Names:    nonNil(d.fields["NAME"]),
			Desc:     d.first("DESC"),
			Sup:      d.fields["SUP"],
			Kind:     ClassStructural, // RFC 4512 default
			Must:     nonNil(d.fields["MUST"]),
			May:      nonNil(d.fields["MAY"]),
			Obsolete: d.has("OBSOLETE"),
			Origin:   d.fields["X-ORIGIN"],
		}
		switch {
		case d.has("ABSTRACT"):
			o.Kind = ClassAbstract
		case d.has("AUXILIARY"):
			o.Kind = ClassAuxiliary
		}
		s.ObjectClasses = append(s.ObjectClasses, o)
		s.classes[strings.ToLower(o.OID)] = o
		for _, n := range o.Names {
			s.classes[strings.ToLower(n)] = o
		}
	}

	s.resolveAttributes()
	return s, errs
}

// resolveAttributes fills inherited syntax/matching rules and computes Kind.
func (s *Schema) resolveAttributes() {
	for _, a := range s.AttributeTypes {
		seen := map[*AttributeType]bool{a: true}
		for p := s.Attr(a.Sup); p != nil && !seen[p]; p = s.Attr(p.Sup) {
			seen[p] = true
			if a.Syntax == "" {
				a.Syntax, a.MaxLen = p.Syntax, p.MaxLen
			}
			if a.Equality == "" {
				a.Equality = p.Equality
			}
			if a.Ordering == "" {
				a.Ordering = p.Ordering
			}
			if a.Substr == "" {
				a.Substr = p.Substr
			}
		}
		a.Kind = kindForSyntax(a.Syntax)
		if passwordAttrs[strings.ToLower(a.Name())] {
			a.Kind = KindPassword
		}
	}
}

// Attr looks up an attribute type by any of its names or its OID
// (case-insensitive). Attribute options such as ";binary" are ignored.
func (s *Schema) Attr(name string) *AttributeType {
	if name == "" {
		return nil
	}
	if i := strings.IndexByte(name, ';'); i >= 0 {
		name = name[:i]
	}
	return s.attrs[strings.ToLower(name)]
}

// Class looks up an object class by name or OID (case-insensitive).
func (s *Schema) Class(name string) *ObjectClass {
	return s.classes[strings.ToLower(name)]
}

// Superclasses returns the class and all its ancestors, nearest first.
func (s *Schema) Superclasses(name string) []*ObjectClass {
	var out []*ObjectClass
	seen := map[*ObjectClass]bool{}
	var walk func(string)
	walk = func(n string) {
		c := s.Class(n)
		if c == nil || seen[c] {
			return
		}
		seen[c] = true
		out = append(out, c)
		for _, sup := range c.Sup {
			walk(sup)
		}
	}
	walk(name)
	return out
}

// Allowed computes the MUST and MAY attributes for a set of object classes,
// including inherited ones. Names are canonical (primary name). MAY excludes
// anything already in MUST. Unknown classes are ignored. extensibleObject
// allows any attribute; callers detect it via AllowsAny.
func (s *Schema) Allowed(objectClasses []string) (must, may []string) {
	mustSet := map[string]bool{}
	maySet := map[string]bool{}
	add := func(set map[string]bool, list *[]string, names []string) {
		for _, n := range names {
			canon := n
			if a := s.Attr(n); a != nil {
				canon = a.Name()
			}
			key := strings.ToLower(canon)
			if !set[key] {
				set[key] = true
				*list = append(*list, canon)
			}
		}
	}
	for _, oc := range objectClasses {
		for _, c := range s.Superclasses(oc) {
			add(mustSet, &must, c.Must)
		}
	}
	for _, oc := range objectClasses {
		for _, c := range s.Superclasses(oc) {
			for _, n := range c.May {
				canon := n
				if a := s.Attr(n); a != nil {
					canon = a.Name()
				}
				if !mustSet[strings.ToLower(canon)] {
					add(maySet, &may, []string{canon})
				}
			}
		}
	}
	slices.SortFunc(must, cmpFold)
	slices.SortFunc(may, cmpFold)
	return nonNil(must), nonNil(may)
}

// AllowsAny reports whether the classes include extensibleObject.
func (s *Schema) AllowsAny(objectClasses []string) bool {
	for _, oc := range objectClasses {
		for _, c := range s.Superclasses(oc) {
			for _, n := range c.Names {
				if strings.EqualFold(n, "extensibleObject") {
					return true
				}
			}
		}
	}
	return false
}

func cmpFold(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) }

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
