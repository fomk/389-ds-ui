// Package entries implements directory data operations for the LDAP editor:
// reading entries, listing children and (later) modifying them. It is generic
// LDAP, made schema-aware through internal/ldapx/schema.
package entries

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
)

// Attribute is one attribute of an entry as sent to the UI.
type Attribute struct {
	Name        string   `json:"name" doc:"Attribute description as returned by the server, including options such as ;lang-ru"`
	Values      []string `json:"values"`
	Binary      bool     `json:"binary,omitempty" doc:"Values are base64-encoded"`
	Redacted    bool     `json:"redacted,omitempty" doc:"Values are password hashes reduced to their {SCHEME}"`
	Operational bool     `json:"operational,omitempty"`
}

// Entry is a directory entry.
type Entry struct {
	DN              string      `json:"dn"`
	RDN             string      `json:"rdn"`
	Parent          string      `json:"parent"`
	ObjectClasses   []string    `json:"objectClasses"`
	Attributes      []Attribute `json:"attributes"`
	NumSubordinates int         `json:"numSubordinates"`
	HasChildren     bool        `json:"hasChildren"`
	ETag            string      `json:"etag" doc:"Version of the user attributes; send as If-Match when modifying"`
}

// Service reads and writes directory entries.
type Service struct {
	ldap     schema.Doer
	schemas  *schema.Cache
	children *childCache
	log      *slog.Logger
}

// New creates a Service.
func New(l schema.Doer, schemas *schema.Cache, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{ldap: l, schemas: schemas, children: newChildCache(30*time.Second, 64), log: log}
}

var (
	// ErrInvalidDN is returned for syntactically invalid DNs.
	ErrInvalidDN = errors.New("invalid DN")
	// ErrInvalidCursor is returned for a malformed paging cursor.
	ErrInvalidCursor = errors.New("invalid cursor")
)

// Get reads one entry with user and operational attributes.
func (s *Service) Get(ctx context.Context, dn string) (*Entry, error) {
	if !ldapx.ValidDN(dn) || dn == "" {
		return nil, ErrInvalidDN
	}
	sch, _, err := s.schemas.Get(ctx, s.ldap)
	if err != nil {
		return nil, err
	}
	var raw *ldap.Entry
	err = s.ldap.Do(ctx, func(c *ldap.Conn) error {
		res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
			1, 0, false, "(objectClass=*)", []string{"*", "+"}, nil))
		if err != nil {
			return err
		}
		if len(res.Entries) == 0 {
			return ldap.NewError(ldap.LDAPResultNoSuchObject, errors.New("entry not found: "+dn))
		}
		raw = res.Entries[0]
		return nil
	})
	if err != nil {
		return nil, err
	}
	return convert(raw, sch), nil
}

func convert(raw *ldap.Entry, sch *schema.Schema) *Entry {
	e := &Entry{DN: raw.DN, ObjectClasses: []string{}, Attributes: []Attribute{}}
	e.RDN, e.Parent = ldapx.SplitRDN(raw.DN)
	for _, a := range raw.Attributes {
		at := sch.Attr(a.Name)
		switch strings.ToLower(a.Name) {
		case "objectclass":
			e.ObjectClasses = append(e.ObjectClasses, a.Values...)
		case "numsubordinates":
			e.NumSubordinates, _ = strconv.Atoi(firstOf(a.Values))
			e.HasChildren = e.HasChildren || e.NumSubordinates > 0
		case "hassubordinates":
			e.HasChildren = e.HasChildren || strings.EqualFold(firstOf(a.Values), "TRUE")
		}
		e.Attributes = append(e.Attributes, encodeAttribute(a, at))
	}
	sortAttributes(e.Attributes)
	e.ETag = etag(raw, sch)
	return e
}

// encodeAttribute decides text vs base64 and redacts password hashes.
func encodeAttribute(a *ldap.EntryAttribute, at *schema.AttributeType) Attribute {
	out := Attribute{Name: a.Name, Values: make([]string, 0, len(a.ByteValues))}
	if at != nil {
		out.Operational = at.Operational()
	}
	kind := schema.KindString
	if at != nil {
		kind = at.Kind
	}
	if kind == schema.KindPassword {
		out.Redacted = true
		for _, v := range a.ByteValues {
			out.Values = append(out.Values, redactPassword(v))
		}
		return out
	}
	out.Binary = kind == schema.KindBinary || hasOption(a.Name, "binary") || !allText(a.ByteValues)
	for _, v := range a.ByteValues {
		if out.Binary {
			out.Values = append(out.Values, base64.StdEncoding.EncodeToString(v))
		} else {
			out.Values = append(out.Values, string(v))
		}
	}
	return out
}

// redactPassword keeps only the "{SCHEME}" prefix of a hashed password.
func redactPassword(v []byte) string {
	s := string(v)
	if strings.HasPrefix(s, "{") {
		if i := strings.IndexByte(s, '}'); i > 0 {
			return s[:i+1]
		}
	}
	return "{CLEAR}"
}

func hasOption(name, opt string) bool {
	parts := strings.Split(name, ";")
	for _, p := range parts[1:] {
		if strings.EqualFold(p, opt) {
			return true
		}
	}
	return false
}

// allText reports whether every value is valid UTF-8 without control
// characters other than tab, CR and LF.
func allText(values [][]byte) bool {
	for _, v := range values {
		if !utf8.Valid(v) {
			return false
		}
		for _, r := range string(v) {
			if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
				return false
			}
		}
	}
	return true
}

// sortAttributes: objectClass first, then user attributes, then operational,
// alphabetically within each group.
func sortAttributes(attrs []Attribute) {
	rank := func(a Attribute) int {
		switch {
		case strings.EqualFold(a.Name, "objectClass"):
			return 0
		case a.Operational:
			return 2
		default:
			return 1
		}
	}
	slices.SortStableFunc(attrs, func(a, b Attribute) int {
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra - rb
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

// etag hashes the user attributes (operational ones change on every read
// for some servers and are not editable anyway).
func etag(raw *ldap.Entry, sch *schema.Schema) string {
	type kv struct {
		name   string
		values []string
	}
	var list []kv
	for _, a := range raw.Attributes {
		if at := sch.Attr(a.Name); at != nil && at.Operational() {
			continue
		}
		vals := make([]string, len(a.ByteValues))
		for i, v := range a.ByteValues {
			vals[i] = string(v)
		}
		slices.Sort(vals)
		list = append(list, kv{strings.ToLower(a.Name), vals})
	}
	slices.SortFunc(list, func(a, b kv) int { return strings.Compare(a.name, b.name) })
	h := sha256.New()
	for _, a := range list {
		h.Write([]byte(a.name))
		for _, v := range a.values {
			h.Write([]byte{0})
			h.Write([]byte(v))
		}
		h.Write([]byte{1})
	}
	return `"` + hex.EncodeToString(h.Sum(nil))[:20] + `"`
}

func firstOf(v []string) string {
	if len(v) > 0 {
		return v[0]
	}
	return ""
}
