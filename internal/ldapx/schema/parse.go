// Package schema parses LDAP subschema definitions (RFC 4512 §4.1) and
// resolves inheritance so the UI can build schema-driven forms.
package schema

import (
	"fmt"
	"strings"
	"unicode"
)

// definition is a parsed "( numericoid key value ... )" description.
// Keys are upper-cased; flag keys (SINGLE-VALUE, ABSTRACT, ...) map to nil.
type definition struct {
	oid    string
	fields map[string][]string
}

func (d definition) has(key string) bool {
	_, ok := d.fields[key]
	return ok
}

func (d definition) first(key string) string {
	if v := d.fields[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// flagKeys never take a value.
var flagKeys = map[string]bool{
	"OBSOLETE": true, "SINGLE-VALUE": true, "COLLECTIVE": true,
	"NO-USER-MODIFICATION": true, "ABSTRACT": true, "STRUCTURAL": true, "AUXILIARY": true,
}

// parseDefinition parses one schema description string.
func parseDefinition(s string) (definition, error) {
	toks, err := tokenize(s)
	if err != nil {
		return definition{}, err
	}
	if len(toks) < 3 || toks[0].val != "(" || toks[len(toks)-1].val != ")" {
		return definition{}, fmt.Errorf("schema definition must be enclosed in parentheses: %q", truncate(s))
	}
	toks = toks[1 : len(toks)-1]
	d := definition{oid: toks[0].String(), fields: map[string][]string{}}
	for i := 1; i < len(toks); {
		key := strings.ToUpper(toks[i].String())
		i++
		if flagKeys[key] {
			d.fields[key] = nil
			continue
		}
		if i >= len(toks) {
			// Unknown trailing keyword: treat as a flag rather than failing.
			d.fields[key] = nil
			break
		}
		if toks[i].String() == "(" && !toks[i].quoted {
			var vals []string
			for i++; i < len(toks) && (toks[i].String() != ")" || toks[i].quoted); i++ {
				if v := toks[i].String(); v != "$" || toks[i].quoted {
					vals = append(vals, v)
				}
			}
			if i >= len(toks) {
				return definition{}, fmt.Errorf("unterminated list for %s in %q", key, truncate(s))
			}
			i++ // skip ")"
			d.fields[key] = vals
			continue
		}
		d.fields[key] = []string{toks[i].String()}
		i++
	}
	return d, nil
}

type token struct {
	val    string
	quoted bool
}

func (t token) String() string { return t.val }

func tokenize(s string) ([]token, error) {
	var toks []token
	r := []rune(s)
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '(' || c == ')' || c == '$':
			toks = append(toks, token{val: string(c)})
			i++
		case c == '\'':
			j := i + 1
			for j < len(r) && r[j] != '\'' {
				j++
			}
			if j >= len(r) {
				return nil, fmt.Errorf("unterminated quoted string in %q", truncate(s))
			}
			toks = append(toks, token{val: unescapeQDString(string(r[i+1 : j])), quoted: true})
			i = j + 1
		default:
			j := i
			for j < len(r) && !unicode.IsSpace(r[j]) && r[j] != '(' && r[j] != ')' && r[j] != '\'' && r[j] != '$' {
				j++
			}
			toks = append(toks, token{val: string(r[i:j])})
			i = j
		}
	}
	return toks, nil
}

// unescapeQDString decodes \27 (') and \5C (\) as defined by RFC 4512 qdstring.
func unescapeQDString(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	r := strings.NewReplacer(`\27`, `'`, `\5C`, `\`, `\5c`, `\`)
	return r.Replace(s)
}

func truncate(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

// splitSyntax separates "1.3.6.1.4.1.1466.115.121.1.15{256}" into OID and length bound.
func splitSyntax(s string) (oid string, maxLen int) {
	oid = s
	if i := strings.IndexByte(s, '{'); i > 0 && strings.HasSuffix(s, "}") {
		oid = s[:i]
		_, _ = fmt.Sscanf(s[i+1:len(s)-1], "%d", &maxLen)
	}
	return oid, maxLen
}
