package entries

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ldapx"
)

// Summary is a light-weight view of an entry for tree nodes.
type Summary struct {
	DN              string   `json:"dn"`
	RDN             string   `json:"rdn"`
	ObjectClasses   []string `json:"objectClasses"`
	NumSubordinates int      `json:"numSubordinates"`
	HasChildren     bool     `json:"hasChildren"`
	Exists          bool     `json:"exists" doc:"False for a naming context whose root entry has not been created yet"`
	System          bool     `json:"system,omitempty" doc:"Server-internal tree such as cn=config"`
}

// ChildrenPage is one page of an entry's children.
type ChildrenPage struct {
	Items      []Summary `json:"items"`
	Total      int       `json:"total"`
	NextCursor string    `json:"nextCursor,omitempty" doc:"Pass as cursor to get the next page"`
	Sorted     bool      `json:"sorted" doc:"Items are sorted by RDN; false for large branches listed in server order"`
}

const (
	// MaxPageSize caps the number of children returned per request.
	MaxPageSize = 1000
	// SortThreshold: branches with at most this many children are read
	// completely and sorted by RDN. Larger ones are paged in server order:
	// reading 100k children takes 389-ds several seconds, a page takes ~20 ms.
	SortThreshold = 2000
)

// ErrPagingExpired: a server-side paging cursor is no longer valid (it is
// bound to the LDAP connection, which may have been re-established).
var ErrPagingExpired = errors.New("the listing expired; reload the branch")

const pagedCursorPrefix = "p:"

var summaryAttrs = []string{"objectClass", "numSubordinates", "hasSubordinates"}

// Children returns a page of the direct children of dn.
//
// Small branches (up to SortThreshold children) are read completely, sorted by
// RDN and cached briefly, so paging is stable and independent of LDAP
// connection state. Large branches are paged by the server (Paged Results) in
// server order; their cursor carries the server's cookie.
func (s *Service) Children(ctx context.Context, dn, cursor string, limit int, refresh bool) (*ChildrenPage, error) {
	if !ldapx.ValidDN(dn) || dn == "" {
		return nil, ErrInvalidDN
	}
	if limit <= 0 || limit > MaxPageSize {
		limit = MaxPageSize
	}
	if strings.HasPrefix(cursor, pagedCursorPrefix) {
		cookie, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(cursor, pagedCursorPrefix))
		if err != nil || len(cookie) == 0 {
			return nil, ErrInvalidCursor
		}
		total, _ := s.countChildren(ctx, dn)
		return s.pagedChildren(ctx, dn, cookie, limit, total)
	}
	offset := 0
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 0 {
			return nil, ErrInvalidCursor
		}
		offset = n
	}

	key := ldapx.NormDN(dn)
	items, ok := s.children.get(key)
	if !ok || refresh {
		total, err := s.countChildren(ctx, dn)
		if err != nil {
			return nil, err
		}
		if total > s.sortThreshold && offset == 0 {
			return s.pagedChildren(ctx, dn, nil, limit, total)
		}
		if items, err = s.fetchChildren(ctx, dn); err != nil {
			return nil, err
		}
		s.children.put(key, items)
	}

	page := &ChildrenPage{Total: len(items), Items: []Summary{}, Sorted: true}
	if offset < len(items) {
		end := min(offset+limit, len(items))
		page.Items = items[offset:end]
		if end < len(items) {
			page.NextCursor = strconv.Itoa(end)
		}
	}
	return page, nil
}

// countChildren reads numSubordinates of dn (-1 if the server does not say).
func (s *Service) countChildren(ctx context.Context, dn string) (int, error) {
	n := -1
	err := s.ldap.Do(ctx, func(c *ldap.Conn) error {
		res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
			1, 0, false, "(objectClass=*)", []string{"numSubordinates"}, nil))
		if err != nil {
			return err
		}
		if len(res.Entries) == 1 {
			if v, err := strconv.Atoi(res.Entries[0].GetAttributeValue("numSubordinates")); err == nil {
				n = v
			}
		}
		return nil
	})
	return n, err
}

// pagedChildren returns one server-side page; cookie is nil for the first one.
func (s *Service) pagedChildren(ctx context.Context, dn string, cookie []byte, limit, total int) (*ChildrenPage, error) {
	page := &ChildrenPage{Items: []Summary{}, Total: total}
	err := s.ldap.Do(ctx, func(c *ldap.Conn) error {
		paging := ldap.NewControlPaging(uint32(limit))
		paging.SetCookie(cookie)
		res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeSingleLevel, ldap.NeverDerefAliases,
			0, 0, false, "(objectClass=*)", summaryAttrs, []ldap.Control{paging}))
		if err != nil {
			if cookie != nil && !ldapx.IsConnError(err) {
				return ErrPagingExpired
			}
			return err
		}
		for _, e := range res.Entries {
			page.Items = append(page.Items, summarize(e))
		}
		if ctrl, ok := ldap.FindControl(res.Controls, ldap.ControlTypePaging).(*ldap.ControlPaging); ok && len(ctrl.Cookie) > 0 {
			page.NextCursor = pagedCursorPrefix + base64.RawURLEncoding.EncodeToString(ctrl.Cookie)
		}
		return nil
	})
	if errors.Is(err, ErrPagingExpired) || (cookie != nil && ldapx.IsConnError(err)) {
		return nil, ErrPagingExpired
	}
	return page, err
}

func (s *Service) fetchChildren(ctx context.Context, dn string) ([]Summary, error) {
	var items []Summary
	err := s.ldap.Do(ctx, func(c *ldap.Conn) error {
		res, err := c.SearchWithPaging(ldap.NewSearchRequest(dn, ldap.ScopeSingleLevel, ldap.NeverDerefAliases,
			0, 0, false, "(objectClass=*)", summaryAttrs, nil), 500)
		if err != nil {
			return err
		}
		items = make([]Summary, 0, len(res.Entries))
		for _, e := range res.Entries {
			items = append(items, summarize(e))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(items, func(a, b Summary) int {
		return strings.Compare(strings.ToLower(a.RDN), strings.ToLower(b.RDN))
	})
	return items, nil
}

func summarize(e *ldap.Entry) Summary {
	s := Summary{DN: e.DN, ObjectClasses: e.GetAttributeValues("objectClass"), Exists: true}
	if s.ObjectClasses == nil {
		s.ObjectClasses = []string{}
	}
	s.RDN, _ = ldapx.SplitRDN(e.DN)
	s.NumSubordinates, _ = strconv.Atoi(e.GetAttributeValue("numSubordinates"))
	s.HasChildren = s.NumSubordinates > 0 || strings.EqualFold(e.GetAttributeValue("hasSubordinates"), "TRUE")
	return s
}

// SystemRoots are server-internal trees shown on request.
var SystemRoots = []string{"cn=config", "cn=schema", "cn=monitor"}

// Roots returns the naming contexts (and optionally the system trees).
func (s *Service) Roots(ctx context.Context, system bool) ([]Summary, error) {
	var out []Summary
	err := s.ldap.Do(ctx, func(c *ldap.Conn) error {
		res, err := c.Search(ldap.NewSearchRequest("", ldap.ScopeBaseObject, ldap.NeverDerefAliases,
			1, 0, false, "(objectClass=*)", []string{"namingContexts"}, nil))
		if err != nil {
			return err
		}
		var dns []string
		if len(res.Entries) == 1 {
			dns = res.Entries[0].GetAttributeValues("namingContexts")
		}
		slices.SortFunc(dns, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
		n := len(dns)
		if system {
			dns = append(dns, SystemRoots...)
		}
		out = make([]Summary, 0, len(dns))
		for i, dn := range dns {
			sum := Summary{DN: dn, RDN: dn, ObjectClasses: []string{}, System: i >= n}
			r, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
				1, 0, false, "(objectClass=*)", summaryAttrs, nil))
			switch {
			case err == nil && len(r.Entries) == 1:
				sum = summarize(r.Entries[0])
				sum.RDN, sum.System = dn, i >= n
			case ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject):
				// Suffix configured, root entry not created yet.
			case err != nil:
				return err
			}
			out = append(out, sum)
		}
		return nil
	})
	return out, err
}

// InvalidateChildren drops the cached children of dn (call after writes).
func (s *Service) InvalidateChildren(dn string) {
	s.children.drop(ldapx.NormDN(dn))
}

// childCache is a small TTL cache of sorted child lists keyed by parent DN.
type childCache struct {
	ttl time.Duration
	max int

	mu    sync.Mutex
	items map[string]childCacheItem
}

type childCacheItem struct {
	list    []Summary
	fetched time.Time
}

func newChildCache(ttl time.Duration, max int) *childCache {
	return &childCache{ttl: ttl, max: max, items: map[string]childCacheItem{}}
}

func (c *childCache) get(key string) ([]Summary, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok || time.Since(it.fetched) > c.ttl {
		return nil, false
	}
	return it.list, true
}

func (c *childCache) put(key string, list []Summary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		// Evict the oldest entry; the cache is tiny, a scan is fine.
		var oldestKey string
		var oldest time.Time
		for k, it := range c.items {
			if oldestKey == "" || it.fetched.Before(oldest) {
				oldestKey, oldest = k, it.fetched
			}
		}
		delete(c.items, oldestKey)
	}
	c.items[key] = childCacheItem{list: list, fetched: time.Now()}
}

func (c *childCache) drop(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// NextNumber returns one more than the largest integer value of attr under
// base (at least floor). Used to suggest uidNumber/gidNumber for new entries;
// it is a suggestion, not a reservation (use the DNA plugin for guarantees).
func (s *Service) NextNumber(ctx context.Context, base, attr string, floor int) (int, error) {
	if !ldapx.ValidDN(base) || base == "" {
		return 0, ErrInvalidDN
	}
	if !attrNameRE.MatchString(attr) {
		return 0, ErrInvalidChange
	}
	maxV := floor - 1
	err := s.ldap.Do(ctx, func(c *ldap.Conn) error {
		res, err := c.SearchWithPaging(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
			0, 0, false, "("+attr+"=*)", []string{attr}, nil), 1000)
		if err != nil {
			return err
		}
		for _, e := range res.Entries {
			for _, v := range e.GetAttributeValues(attr) {
				if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > maxV {
					maxV = n
				}
			}
		}
		return nil
	})
	return maxV + 1, err
}

var attrNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)

func (c *childCache) clear() {
	c.mu.Lock()
	c.items = map[string]childCacheItem{}
	c.mu.Unlock()
}
