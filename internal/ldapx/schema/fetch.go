package schema

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// Fetch reads the subschema entry advertised by the root DSE (cn=schema on
// 389-ds). The returned version is a content hash, usable as an ETag: 389-ds
// exposes neither modifyTimestamp nor nsSchemaCSN on cn=schema.
func Fetch(conn *ldap.Conn) (s *Schema, version string, parseErrs []error, err error) {
	dn := "cn=schema"
	if res, err := conn.Search(ldap.NewSearchRequest("", ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{"subschemaSubentry"}, nil)); err == nil && len(res.Entries) == 1 {
		if v := res.Entries[0].GetAttributeValue("subschemaSubentry"); v != "" {
			dn = v
		}
	}
	res, err := conn.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=subschema)",
		[]string{"attributeTypes", "objectClasses", "ldapSyntaxes"}, nil))
	if err != nil {
		return nil, "", nil, err
	}
	if len(res.Entries) != 1 {
		return nil, "", nil, errors.New("subschema entry " + dn + " not found")
	}
	e := res.Entries[0]
	raw := Raw{
		AttributeTypes: e.GetAttributeValues("attributeTypes"),
		ObjectClasses:  e.GetAttributeValues("objectClasses"),
		Syntaxes:       e.GetAttributeValues("ldapSyntaxes"),
	}
	s, parseErrs = Parse(raw)
	return s, hashRaw(raw), parseErrs, nil
}

func hashRaw(r Raw) string {
	h := sha256.New()
	for _, list := range [][]string{r.AttributeTypes, r.ObjectClasses, r.Syntaxes} {
		for _, v := range list {
			h.Write([]byte(v))
			h.Write([]byte{0})
		}
		h.Write([]byte{1})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Doer runs a function with an LDAP connection (implemented by ldapx.Client).
type Doer interface {
	Do(ctx context.Context, fn func(*ldap.Conn) error) error
}

// Cache keeps the parsed schema for TTL. The schema changes rarely; callers
// that modify it (schema editor) call Invalidate.
type Cache struct {
	ttl time.Duration
	log *slog.Logger

	mu      sync.Mutex
	schema  *Schema
	version string
	fetched time.Time
}

// NewCache creates a cache with the given time-to-live.
func NewCache(ttl time.Duration, log *slog.Logger) *Cache {
	if log == nil {
		log = slog.Default()
	}
	return &Cache{ttl: ttl, log: log}
}

// Get returns the cached schema, refetching it if it is older than the TTL.
// If a refetch fails but an older copy exists, the old copy is returned.
func (c *Cache) Get(ctx context.Context, l Doer) (*Schema, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.schema != nil && time.Since(c.fetched) < c.ttl {
		return c.schema, c.version, nil
	}
	var (
		s         *Schema
		version   string
		parseErrs []error
	)
	err := l.Do(ctx, func(conn *ldap.Conn) (err error) {
		s, version, parseErrs, err = Fetch(conn)
		return err
	})
	if err != nil {
		if c.schema != nil {
			c.log.Warn("schema refresh failed, using cached copy", "err", err)
			return c.schema, c.version, nil
		}
		return nil, "", err
	}
	for _, e := range parseErrs {
		c.log.Warn("skipping unparsable schema definition", "err", e)
	}
	c.schema, c.version, c.fetched = s, version, time.Now()
	return s, version, nil
}

// Invalidate forces the next Get to refetch.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	c.fetched = time.Time{}
	c.mu.Unlock()
}
