package entries

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ldapx"
)

var (
	// ErrPreconditionFailed means the entry changed since the client read it.
	ErrPreconditionFailed = errors.New("the entry was modified by someone else; reload it and try again")
	// ErrInvalidChange is returned for malformed modification requests.
	ErrInvalidChange = errors.New("invalid change")
	// ErrSubtreeTooLarge protects against accidental mass deletion in one request.
	ErrSubtreeTooLarge = errors.New("subtree is too large to delete in one request")
)

// MaxSubtreeDelete caps the number of entries deleted by one request.
const MaxSubtreeDelete = 10000

// Change is one modification of an attribute.
type Change struct {
	Op     string   `json:"op" enum:"add,delete,replace" doc:"delete without values removes the attribute"`
	Attr   string   `json:"attr" minLength:"1"`
	Values []string `json:"values,omitempty"`
	Binary bool     `json:"binary,omitempty" doc:"Values are base64-encoded"`
}

// Modify applies changes to dn. If ifMatch is non-empty, it must equal the
// entry's current ETag, otherwise ErrPreconditionFailed is returned.
func (s *Service) Modify(ctx context.Context, dn, ifMatch string, changes []Change) (*Entry, error) {
	if !ldapx.ValidDN(dn) || dn == "" {
		return nil, ErrInvalidDN
	}
	if len(changes) == 0 {
		return nil, fmt.Errorf("%w: no changes", ErrInvalidChange)
	}
	req := ldap.NewModifyRequest(dn, nil)
	for _, c := range changes {
		vals, err := decodeValues(c)
		if err != nil {
			return nil, err
		}
		switch c.Op {
		case "add":
			if len(vals) == 0 {
				return nil, fmt.Errorf("%w: add %s without values", ErrInvalidChange, c.Attr)
			}
			req.Add(c.Attr, vals)
		case "delete":
			req.Delete(c.Attr, vals)
		case "replace":
			req.Replace(c.Attr, vals)
		default:
			return nil, fmt.Errorf("%w: unknown op %q", ErrInvalidChange, c.Op)
		}
	}
	if ifMatch != "" {
		cur, err := s.Get(ctx, dn)
		if err != nil {
			return nil, err
		}
		if cur.ETag != ifMatch {
			return nil, ErrPreconditionFailed
		}
	}
	if err := s.ldap.Do(ctx, func(c *ldap.Conn) error { return c.Modify(req) }); err != nil {
		return nil, err
	}
	_, parent := ldapx.SplitRDN(dn)
	s.InvalidateChildren(parent) // objectClass changes affect the tree icon
	return s.Get(ctx, dn)
}

func decodeValues(c Change) ([]string, error) {
	if !c.Binary {
		return nonNilStrings(c.Values), nil
	}
	out := make([]string, len(c.Values))
	for i, v := range c.Values {
		b, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: value %d is not valid base64", ErrInvalidChange, c.Attr, i+1)
		}
		out[i] = string(b)
	}
	return out, nil
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// NewEntry is a request to create an entry.
type NewEntry struct {
	DN         string      `json:"dn" minLength:"1"`
	Attributes []NewValues `json:"attributes" minItems:"1"`
}

// NewValues are the values of one attribute of a new entry.
type NewValues struct {
	Name   string   `json:"name" minLength:"1"`
	Values []string `json:"values" minItems:"1"`
	Binary bool     `json:"binary,omitempty"`
}

// Add creates an entry and returns it as stored by the server.
func (s *Service) Add(ctx context.Context, ne NewEntry) (*Entry, error) {
	if !ldapx.ValidDN(ne.DN) || ne.DN == "" {
		return nil, ErrInvalidDN
	}
	req := ldap.NewAddRequest(ne.DN, nil)
	for _, a := range ne.Attributes {
		vals, err := decodeValues(Change{Attr: a.Name, Values: a.Values, Binary: a.Binary})
		if err != nil {
			return nil, err
		}
		req.Attribute(a.Name, vals)
	}
	if err := s.ldap.Do(ctx, func(c *ldap.Conn) error { return c.Add(req) }); err != nil {
		return nil, err
	}
	_, parent := ldapx.SplitRDN(ne.DN)
	s.InvalidateChildren(parent)
	return s.Get(ctx, ne.DN)
}

// DeleteResult reports how many entries were removed.
type DeleteResult struct {
	Deleted int `json:"deleted"`
}

// Delete removes dn. With subtree=true its descendants are removed first,
// deepest entries first (389-ds does not support the Tree Delete control).
func (s *Service) Delete(ctx context.Context, dn string, subtree bool) (*DeleteResult, error) {
	if !ldapx.ValidDN(dn) || dn == "" {
		return nil, ErrInvalidDN
	}
	res := &DeleteResult{}
	err := s.ldap.Do(ctx, func(c *ldap.Conn) error {
		targets := []string{dn}
		if subtree {
			sr, err := c.SearchWithPaging(ldap.NewSearchRequest(dn, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
				0, 0, false, "(objectClass=*)", []string{"1.1"}, nil), 500)
			if err != nil {
				return err
			}
			if len(sr.Entries) > MaxSubtreeDelete {
				return fmt.Errorf("%w: %d entries (limit %d)", ErrSubtreeTooLarge, len(sr.Entries), MaxSubtreeDelete)
			}
			targets = targets[:0]
			for _, e := range sr.Entries {
				targets = append(targets, e.DN)
			}
			// Children before parents: more RDN components = deeper.
			slices.SortStableFunc(targets, func(a, b string) int { return depth(b) - depth(a) })
		}
		for _, t := range targets {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := c.Del(ldap.NewDelRequest(t, nil)); err != nil {
				if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) && t != dn {
					continue // removed concurrently
				}
				return fmt.Errorf("delete %s: %w", t, err)
			}
			res.Deleted++
		}
		return nil
	})
	if res.Deleted > 0 {
		_, parent := ldapx.SplitRDN(dn)
		s.InvalidateChildren(parent)
		s.InvalidateChildren(dn)
	}
	if err != nil {
		return res, err
	}
	return res, nil
}

func depth(dn string) int {
	n := 0
	for rest := dn; rest != ""; n++ {
		_, rest = ldapx.SplitRDN(rest)
	}
	return n
}

// RenameRequest renames and/or moves an entry.
type RenameRequest struct {
	DN           string `json:"dn" minLength:"1"`
	NewRDN       string `json:"newRdn" minLength:"1" doc:"New RDN, e.g. uid=newname (may equal the current RDN when only moving)"`
	DeleteOldRDN bool   `json:"deleteOldRdn" doc:"Remove the old RDN value from the entry"`
	NewParent    string `json:"newParent,omitempty" doc:"New parent DN; empty keeps the current parent"`
}

// RenameResult is the entry's new DN.
type RenameResult struct {
	DN string `json:"dn"`
}

// Rename performs a ModifyDN operation.
func (s *Service) Rename(ctx context.Context, r RenameRequest) (*RenameResult, error) {
	if !ldapx.ValidDN(r.DN) || r.DN == "" || !ldapx.ValidDN(r.NewRDN) || strings.Contains(stripEscapes(r.NewRDN), ",") {
		return nil, ErrInvalidDN
	}
	if r.NewParent != "" && !ldapx.ValidDN(r.NewParent) {
		return nil, ErrInvalidDN
	}
	_, oldParent := ldapx.SplitRDN(r.DN)
	newParent := oldParent
	if r.NewParent != "" && ldapx.NormDN(r.NewParent) != ldapx.NormDN(oldParent) {
		newParent = r.NewParent
	}
	req := ldap.NewModifyDNRequest(r.DN, r.NewRDN, r.DeleteOldRDN, "")
	if newParent != oldParent {
		req.NewSuperior = newParent
	}
	if err := s.ldap.Do(ctx, func(c *ldap.Conn) error { return c.ModifyDN(req) }); err != nil {
		return nil, err
	}
	s.InvalidateChildren(oldParent)
	s.InvalidateChildren(newParent)
	newDN := r.NewRDN
	if newParent != "" {
		newDN += "," + newParent
	}
	return &RenameResult{DN: newDN}, nil
}

// stripEscapes removes escaped characters so that structural commas can be found.
func stripEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// PasswordRequest sets a user's password with the Password Modify extended operation.
type PasswordRequest struct {
	DN       string `json:"dn" minLength:"1"`
	Password string `json:"password,omitempty" doc:"New password; empty asks the server to generate one"`
}

// PasswordResult carries a server-generated password, if one was requested.
type PasswordResult struct {
	Generated string `json:"generated,omitempty"`
}

// ErrPasswordGenerationUnavailable: the server only generates passwords via
// the extended operation, which 389-ds refuses on unencrypted connections.
var ErrPasswordGenerationUnavailable = errors.New("the server can generate a password only over a secure connection (LDAPI or TLS); enter a password instead")

// SetPassword uses the Password Modify extended operation (RFC 3062) so that
// the server applies its password policy and hashing. 389-ds accepts it only
// on secure connections (LDAPI, TLS); on plain LDAP it falls back to replacing
// userPassword, which the server hashes as well.
func (s *Service) SetPassword(ctx context.Context, r PasswordRequest) (*PasswordResult, error) {
	if !ldapx.ValidDN(r.DN) || r.DN == "" {
		return nil, ErrInvalidDN
	}
	var res *ldap.PasswordModifyResult
	err := s.ldap.Do(ctx, func(c *ldap.Conn) (err error) {
		res, err = c.PasswordModify(ldap.NewPasswordModifyRequest(r.DN, "", r.Password))
		return err
	})
	if ldap.IsErrorWithCode(err, ldap.LDAPResultConfidentialityRequired) {
		if r.Password == "" {
			return nil, ErrPasswordGenerationUnavailable
		}
		req := ldap.NewModifyRequest(r.DN, nil)
		req.Replace("userPassword", []string{r.Password})
		if err := s.ldap.Do(ctx, func(c *ldap.Conn) error { return c.Modify(req) }); err != nil {
			return nil, err
		}
		return &PasswordResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &PasswordResult{Generated: res.GeneratedPassword}, nil
}
