package ds

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

const dnMappingTree = "cn=mapping tree,cn=config"

// Backend is a database backend with its mapping tree settings.
type Backend struct {
	Name           string `json:"name"`
	Suffix         string `json:"suffix"`
	ReadOnly       bool   `json:"readOnly"`
	RequireIndex   bool   `json:"requireIndex" doc:"Reject unindexed searches"`
	State          string `json:"state" enum:"backend,disabled,referral,referral on update," doc:"Mapping tree state"`
	Referral       string `json:"referral,omitempty"`
	ParentSuffix   string `json:"parentSuffix,omitempty" doc:"Suffix this one is a sub-suffix of (nsslapd-parent-suffix)"`
	CacheMemSize   int64  `json:"cacheMemSize" doc:"Entry cache, bytes (nsslapd-cachememsize)"`
	DNCacheMemSize int64  `json:"dnCacheMemSize" doc:"DN cache, bytes"`
	RootExists     bool   `json:"rootExists" doc:"The suffix root entry exists"`
}

var (
	// ErrInvalidName is returned for a backend or index name that is not allowed.
	ErrInvalidName = errors.New("invalid name")
	// ErrNotFound is returned for an unknown backend or index.
	ErrNotFound = errors.New("not found")
	// ErrHasSubSuffixes protects a suffix other suffixes hang under.
	ErrHasSubSuffixes = errors.New("the suffix has sub-suffixes")
	// ErrSystemIndex protects indexes the server needs.
	ErrSystemIndex = errors.New("system indexes cannot be deleted")
)

var backendNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

func backendDN(name string) string { return "cn=" + ldap.EscapeDN(name) + "," + DNLDBM }

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// ListBackends returns all LDBM backends.
func ListBackends(c *ldap.Conn) ([]Backend, error) {
	res, err := c.Search(ldap.NewSearchRequest(DNLDBM, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=nsBackendInstance)", []string{"cn", "nsslapd-suffix", "nsslapd-readonly", "nsslapd-require-index",
			"nsslapd-cachememsize", "nsslapd-dncachememsize"}, nil))
	if err != nil {
		return nil, err
	}
	mt, err := c.Search(ldap.NewSearchRequest(dnMappingTree, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=nsMappingTree)", []string{"nsslapd-backend", "nsslapd-state", "nsslapd-referral", "nsslapd-parent-suffix"}, nil))
	if err != nil {
		return nil, err
	}
	mapping := map[string]*ldap.Entry{}
	for _, e := range mt.Entries {
		mapping[strings.ToLower(e.GetAttributeValue("nsslapd-backend"))] = e
	}
	out := make([]Backend, 0, len(res.Entries))
	for _, e := range res.Entries {
		b := Backend{
			Name:           e.GetAttributeValue("cn"),
			Suffix:         e.GetAttributeValue("nsslapd-suffix"),
			ReadOnly:       strings.EqualFold(e.GetAttributeValue("nsslapd-readonly"), "on"),
			RequireIndex:   strings.EqualFold(e.GetAttributeValue("nsslapd-require-index"), "on"),
			CacheMemSize:   num(e, "nsslapd-cachememsize"),
			DNCacheMemSize: num(e, "nsslapd-dncachememsize"),
		}
		if m := mapping[strings.ToLower(b.Name)]; m != nil {
			b.State = m.GetAttributeValue("nsslapd-state")
			b.Referral = m.GetAttributeValue("nsslapd-referral")
			b.ParentSuffix = m.GetAttributeValue("nsslapd-parent-suffix")
		}
		if _, err := readEntry(c, b.Suffix, "1.1"); err == nil {
			b.RootExists = true
		}
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b Backend) int { return strings.Compare(a.Suffix, b.Suffix) })
	return out, nil
}

// GetBackend returns one backend by name.
func GetBackend(c *ldap.Conn, name string) (*Backend, error) {
	list, err := ListBackends(c)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if strings.EqualFold(list[i].Name, name) {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("%w: backend %s", ErrNotFound, name)
}

// NewBackend is a request to create a suffix.
type NewBackend struct {
	Name       string `json:"name" minLength:"1" doc:"Backend (database) name, e.g. userroot"`
	Suffix     string `json:"suffix" minLength:"1" doc:"e.g. dc=example,dc=org"`
	CreateRoot bool   `json:"createRoot" doc:"Also create the suffix root entry"`
	// ParentSuffix makes a sub-suffix: its entries appear under the parent
	// suffix in the tree although they live in their own backend.
	ParentSuffix string `json:"parentSuffix,omitempty" doc:"Existing suffix to create a sub-suffix of; the suffix must be below it"`
}

// sameDN compares two DNs ignoring case and spacing.
func sameDN(a, b string) bool {
	pa, err1 := ldap.ParseDN(a)
	pb, err2 := ldap.ParseDN(b)
	return err1 == nil && err2 == nil && pa.EqualFold(pb)
}

// below reports whether dn is strictly below base.
func below(dn, base string) bool {
	pd, err1 := ldap.ParseDN(dn)
	pb, err2 := ldap.ParseDN(base)
	return err1 == nil && err2 == nil && len(pd.RDNs) > len(pb.RDNs) && pb.AncestorOfFold(pd)
}

// CreateBackend creates the backend, its mapping tree entry and optionally the
// root entry, in that order (the order lib389 and the server expect).
func CreateBackend(c *ldap.Conn, nb NewBackend) error {
	if !backendNameRE.MatchString(nb.Name) {
		return fmt.Errorf("%w: backend name %q (letters, digits, - and _)", ErrInvalidName, nb.Name)
	}
	if _, err := ldap.ParseDN(nb.Suffix); err != nil || nb.Suffix == "" {
		return fmt.Errorf("%w: suffix %q", ErrInvalidName, nb.Suffix)
	}
	if nb.ParentSuffix != "" {
		list, err := ListBackends(c)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(list, func(b Backend) bool { return sameDN(b.Suffix, nb.ParentSuffix) })
		if i < 0 {
			return fmt.Errorf("%w: parent suffix %s", ErrNotFound, nb.ParentSuffix)
		}
		if !below(nb.Suffix, list[i].Suffix) {
			return fmt.Errorf("%w: %s is not below the parent suffix %s", ErrInvalidName, nb.Suffix, list[i].Suffix)
		}
		nb.ParentSuffix = list[i].Suffix // as the server spells it
	}
	be := ldap.NewAddRequest(backendDN(nb.Name), nil)
	be.Attribute("objectClass", []string{"top", "extensibleObject", "nsBackendInstance"})
	be.Attribute("cn", []string{nb.Name})
	be.Attribute("nsslapd-suffix", []string{nb.Suffix})
	if err := c.Add(be); err != nil {
		return fmt.Errorf("create backend: %w", err)
	}
	mt := ldap.NewAddRequest("cn="+ldap.EscapeDN(nb.Suffix)+","+dnMappingTree, nil)
	mt.Attribute("objectClass", []string{"top", "extensibleObject", "nsMappingTree"})
	mt.Attribute("cn", []string{nb.Suffix})
	mt.Attribute("nsslapd-state", []string{"backend"})
	mt.Attribute("nsslapd-backend", []string{nb.Name})
	if nb.ParentSuffix != "" {
		mt.Attribute("nsslapd-parent-suffix", []string{nb.ParentSuffix})
	}
	if err := c.Add(mt); err != nil {
		_ = c.Del(ldap.NewDelRequest(backendDN(nb.Name), nil)) // roll back
		return fmt.Errorf("create mapping tree: %w", err)
	}
	if nb.CreateRoot {
		if err := c.Add(rootEntry(nb.Suffix)); err != nil {
			return fmt.Errorf("create root entry: %w", err)
		}
	}
	return nil
}

// rootEntry builds a minimal entry for a suffix from its first RDN.
func rootEntry(suffix string) *ldap.AddRequest {
	req := ldap.NewAddRequest(suffix, nil)
	p, _ := ldap.ParseDN(suffix)
	attr, value := "", ""
	if p != nil && len(p.RDNs) > 0 && len(p.RDNs[0].Attributes) > 0 {
		attr, value = p.RDNs[0].Attributes[0].Type, p.RDNs[0].Attributes[0].Value
	}
	switch strings.ToLower(attr) {
	case "dc":
		req.Attribute("objectClass", []string{"top", "domain"})
	case "o":
		req.Attribute("objectClass", []string{"top", "organization"})
	case "ou":
		req.Attribute("objectClass", []string{"top", "organizationalUnit"})
	case "c":
		req.Attribute("objectClass", []string{"top", "country"})
	default:
		req.Attribute("objectClass", []string{"top", "extensibleObject"})
	}
	if attr != "" {
		req.Attribute(attr, []string{value})
	}
	return req
}

// DeleteBackend removes the mapping tree entry and the backend (with its data).
func DeleteBackend(c *ldap.Conn, name string) error {
	list, err := ListBackends(c)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(list, func(b Backend) bool { return strings.EqualFold(b.Name, name) })
	if i < 0 {
		return fmt.Errorf("%w: backend %s", ErrNotFound, name)
	}
	b := list[i]
	var subs []string
	for _, o := range list {
		if o.ParentSuffix != "" && sameDN(o.ParentSuffix, b.Suffix) {
			subs = append(subs, o.Suffix)
		}
	}
	if len(subs) > 0 {
		return fmt.Errorf("%w: delete %s first", ErrHasSubSuffixes, strings.Join(subs, "; "))
	}
	mt, err := c.Search(ldap.NewSearchRequest(dnMappingTree, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(nsslapd-backend="+ldap.EscapeFilter(b.Name)+")", []string{"1.1"}, nil))
	if err != nil {
		return err
	}
	for _, e := range mt.Entries {
		if err := c.Del(ldap.NewDelRequest(e.DN, nil)); err != nil {
			return fmt.Errorf("delete mapping tree: %w", err)
		}
	}
	// The backend's config entry has children (indexes, encryption settings,
	// monitor) and the server refuses to delete a non-leaf, so remove the
	// config subtree deepest first. Children the server manages itself may
	// refuse deletion; only the backend entry itself must go.
	sub, err := c.SearchWithPaging(ldap.NewSearchRequest(backendDN(b.Name), ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, "(objectClass=*)", []string{"1.1"}, nil), 500)
	if err != nil {
		return err
	}
	dns := make([]string, 0, len(sub.Entries))
	for _, e := range sub.Entries {
		dns = append(dns, e.DN)
	}
	slices.SortStableFunc(dns, func(a, b string) int { return strings.Count(b, ",") - strings.Count(a, ",") })
	top := strings.ToLower(backendDN(b.Name))
	for _, dn := range dns {
		err := c.Del(ldap.NewDelRequest(dn, nil))
		if err != nil && strings.EqualFold(dn, top) {
			return fmt.Errorf("delete backend: %w", err)
		}
	}
	return nil
}

// BackendPatch changes backend settings; nil fields are left alone.
type BackendPatch struct {
	ReadOnly       *bool   `json:"readOnly,omitempty"`
	RequireIndex   *bool   `json:"requireIndex,omitempty"`
	State          *string `json:"state,omitempty" enum:"backend,disabled,referral,referral on update"`
	Referral       *string `json:"referral,omitempty" doc:"LDAP URL; empty removes it"`
	CacheMemSize   *int64  `json:"cacheMemSize,omitempty" minimum:"512000"`
	DNCacheMemSize *int64  `json:"dnCacheMemSize,omitempty" minimum:"512000"`
}

// UpdateBackend applies a patch.
func UpdateBackend(c *ldap.Conn, name string, p BackendPatch) error {
	b, err := GetBackend(c, name)
	if err != nil {
		return err
	}
	be := ldap.NewModifyRequest(backendDN(b.Name), nil)
	if p.ReadOnly != nil {
		be.Replace("nsslapd-readonly", []string{onOff(*p.ReadOnly)})
	}
	if p.RequireIndex != nil {
		be.Replace("nsslapd-require-index", []string{onOff(*p.RequireIndex)})
	}
	if p.CacheMemSize != nil {
		be.Replace("nsslapd-cachememsize", []string{strconv.FormatInt(*p.CacheMemSize, 10)})
	}
	if p.DNCacheMemSize != nil {
		be.Replace("nsslapd-dncachememsize", []string{strconv.FormatInt(*p.DNCacheMemSize, 10)})
	}
	if len(be.Changes) > 0 {
		if err := c.Modify(be); err != nil {
			return err
		}
	}
	if p.State == nil && p.Referral == nil {
		return nil
	}
	mt, err := c.Search(ldap.NewSearchRequest(dnMappingTree, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(nsslapd-backend="+ldap.EscapeFilter(b.Name)+")", []string{"1.1"}, nil))
	if err != nil {
		return err
	}
	if len(mt.Entries) == 0 {
		return fmt.Errorf("%w: mapping tree of %s", ErrNotFound, b.Name)
	}
	m := ldap.NewModifyRequest(mt.Entries[0].DN, nil)
	if p.Referral != nil {
		if *p.Referral == "" {
			if b.Referral != "" {
				m.Delete("nsslapd-referral", nil)
			}
		} else {
			m.Replace("nsslapd-referral", []string{*p.Referral})
		}
	}
	if p.State != nil {
		m.Replace("nsslapd-state", []string{*p.State})
	}
	if len(m.Changes) == 0 {
		return nil
	}
	return c.Modify(m)
}

// --- Indexes -----------------------------------------------------------------

// Index is an attribute index of a backend.
type Index struct {
	Attribute     string   `json:"attribute"`
	Types         []string `json:"types" doc:"eq, pres, sub, approx"`
	MatchingRules []string `json:"matchingRules"`
	System        bool     `json:"system" doc:"Required by the server; cannot be deleted"`
}

var indexTypes = []string{"eq", "pres", "sub", "approx"}

func indexContainer(backend string) string { return "cn=index," + backendDN(backend) }

// DNDefaultIndexes holds the indexes every new backend starts with.
const DNDefaultIndexes = "cn=default indexes,cn=config," + DNLDBM

// ListIndexes returns the indexes of a backend sorted by attribute.
func ListIndexes(c *ldap.Conn, backend string) ([]Index, error) {
	// cn=config answers a search under a missing base with success and no
	// entries instead of noSuchObject, so check the backend explicitly.
	if _, err := readEntry(c, backendDN(backend), "1.1"); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			return nil, fmt.Errorf("%w: backend %s", ErrNotFound, backend)
		}
		return nil, err
	}
	list, err := listIndexesIn(c, indexContainer(backend))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil, fmt.Errorf("%w: backend %s", ErrNotFound, backend)
	}
	return list, err
}

// ListDefaultIndexes returns the default indexes for new backends.
func ListDefaultIndexes(c *ldap.Conn) ([]Index, error) { return listIndexesIn(c, DNDefaultIndexes) }

func listIndexesIn(c *ldap.Conn, container string) ([]Index, error) {
	res, err := c.Search(ldap.NewSearchRequest(container, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=nsIndex)", []string{"cn", "nsIndexType", "nsMatchingRule", "nsSystemIndex"}, nil))
	if err != nil {
		return nil, err
	}
	out := make([]Index, 0, len(res.Entries))
	for _, e := range res.Entries {
		ix := Index{
			Attribute:     e.GetAttributeValue("cn"),
			Types:         e.GetAttributeValues("nsIndexType"),
			MatchingRules: e.GetAttributeValues("nsMatchingRule"),
			System:        strings.EqualFold(e.GetAttributeValue("nsSystemIndex"), "true"),
		}
		if ix.Types == nil {
			ix.Types = []string{}
		}
		if ix.MatchingRules == nil {
			ix.MatchingRules = []string{}
		}
		out = append(out, ix)
	}
	slices.SortFunc(out, func(a, b Index) int {
		return strings.Compare(strings.ToLower(a.Attribute), strings.ToLower(b.Attribute))
	})
	return out, nil
}

var attrRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)

func validIndex(ix Index) error {
	if !attrRE.MatchString(ix.Attribute) {
		return fmt.Errorf("%w: attribute %q", ErrInvalidName, ix.Attribute)
	}
	if len(ix.Types) == 0 {
		return fmt.Errorf("%w: at least one index type is needed", ErrInvalidName)
	}
	for _, t := range ix.Types {
		if !slices.Contains(indexTypes, t) {
			return fmt.Errorf("%w: index type %q", ErrInvalidName, t)
		}
	}
	return nil
}

// CreateIndex adds an index definition. The index is empty until reindexed.
func CreateIndex(c *ldap.Conn, backend string, ix Index) error {
	return createIndexIn(c, indexContainer(backend), ix)
}

// CreateDefaultIndex adds a default index (used by backends created later).
func CreateDefaultIndex(c *ldap.Conn, ix Index) error { return createIndexIn(c, DNDefaultIndexes, ix) }

func createIndexIn(c *ldap.Conn, container string, ix Index) error {
	if err := validIndex(ix); err != nil {
		return err
	}
	req := ldap.NewAddRequest("cn="+ldap.EscapeDN(ix.Attribute)+","+container, nil)
	req.Attribute("objectClass", []string{"top", "nsIndex"})
	req.Attribute("cn", []string{ix.Attribute})
	req.Attribute("nsSystemIndex", []string{"false"})
	req.Attribute("nsIndexType", ix.Types)
	if len(ix.MatchingRules) > 0 {
		req.Attribute("nsMatchingRule", ix.MatchingRules)
	}
	return c.Add(req)
}

// UpdateIndex replaces the types and matching rules of an index.
func UpdateIndex(c *ldap.Conn, backend string, ix Index) error {
	return updateIndexIn(c, indexContainer(backend), ix)
}

// UpdateDefaultIndex replaces the types and matching rules of a default index.
func UpdateDefaultIndex(c *ldap.Conn, ix Index) error { return updateIndexIn(c, DNDefaultIndexes, ix) }

func updateIndexIn(c *ldap.Conn, container string, ix Index) error {
	if err := validIndex(ix); err != nil {
		return err
	}
	req := ldap.NewModifyRequest("cn="+ldap.EscapeDN(ix.Attribute)+","+container, nil)
	req.Replace("nsIndexType", ix.Types)
	req.Replace("nsMatchingRule", ix.MatchingRules)
	return c.Modify(req)
}

// DeleteIndex removes a non-system index.
func DeleteIndex(c *ldap.Conn, backend, attr string) error {
	list, err := ListIndexes(c, backend)
	if err != nil {
		return err
	}
	return deleteIndexIn(c, indexContainer(backend), list, attr)
}

// DeleteDefaultIndex removes a non-system default index.
func DeleteDefaultIndex(c *ldap.Conn, attr string) error {
	list, err := ListDefaultIndexes(c)
	if err != nil {
		return err
	}
	return deleteIndexIn(c, DNDefaultIndexes, list, attr)
}

func deleteIndexIn(c *ldap.Conn, container string, list []Index, attr string) error {
	for _, ix := range list {
		if strings.EqualFold(ix.Attribute, attr) {
			if ix.System {
				return ErrSystemIndex
			}
			return c.Del(ldap.NewDelRequest("cn="+ldap.EscapeDN(ix.Attribute)+","+container, nil))
		}
	}
	return fmt.Errorf("%w: index %s", ErrNotFound, attr)
}

// ReindexAttrs builds the attributes of an index task for a backend.
// With no attributes and no VLV indexes, every index of the backend is
// rebuilt; vlv names VLV searches whose sort indexes are rebuilt.
func ReindexAttrs(c *ldap.Conn, backend string, attrs, vlv []string) (map[string][]string, error) {
	if len(vlv) > 0 {
		all, err := ListVLV(c, backend)
		if err != nil {
			return nil, err
		}
		var names []string
		for _, v := range vlv {
			i := slices.IndexFunc(all, func(x VLVIndex) bool { return strings.EqualFold(x.Name, v) })
			if i < 0 {
				return nil, fmt.Errorf("%w: VLV index %s", ErrNotFound, v)
			}
			names = append(names, all[i].IndexNames...)
		}
		task := map[string][]string{"nsInstance": {backend}, "nsIndexVLVAttribute": names}
		if len(attrs) > 0 {
			task["nsIndexAttribute"] = attrs
		}
		return task, nil
	}
	if len(attrs) == 0 {
		list, err := ListIndexes(c, backend)
		if err != nil {
			return nil, err
		}
		for _, ix := range list {
			attrs = append(attrs, ix.Attribute)
		}
	}
	for _, a := range attrs {
		if !attrRE.MatchString(a) {
			return nil, fmt.Errorf("%w: attribute %q", ErrInvalidName, a)
		}
	}
	return map[string][]string{"nsInstance": {backend}, "nsIndexAttribute": attrs}, nil
}

// --- VLV indexes -------------------------------------------------------------

// VLVIndex is a virtual list view search with its sort index.
type VLVIndex struct {
	Name   string   `json:"name"`
	Base   string   `json:"base"`
	Scope  string   `json:"scope" enum:"base,one,sub"`
	Filter string   `json:"filter"`
	Sorts  []string `json:"sorts" doc:"vlvSort values of the child vlvIndex entries"`
	// cn of the vlvIndex entries, which the index task names.
	IndexNames []string `json:"indexNames"`
}

// NewVLV is a VLV index to create.
type NewVLV struct {
	Name   string   `json:"name" minLength:"1"`
	Base   string   `json:"base" minLength:"1"`
	Scope  string   `json:"scope" enum:"base,one,sub"`
	Filter string   `json:"filter" minLength:"1"`
	Sorts  []string `json:"sorts" minItems:"1" doc:"One sort index per value: attributes separated by spaces, -attr for descending"`
}

// ListVLV returns the VLV searches of a backend.
func ListVLV(c *ldap.Conn, backend string) ([]VLVIndex, error) {
	res, err := c.Search(ldap.NewSearchRequest(backendDN(backend), ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(|(objectClass=vlvSearch)(objectClass=vlvIndex))", []string{"cn", "vlvBase", "vlvScope", "vlvFilter", "vlvSort", "objectClass"}, nil))
	if err != nil {
		return nil, err
	}
	searches := map[string]*VLVIndex{}
	var order []string
	for _, e := range res.Entries {
		if slices.ContainsFunc(e.GetAttributeValues("objectClass"), func(s string) bool { return strings.EqualFold(s, "vlvSearch") }) {
			scope := map[string]string{"0": "base", "1": "one", "2": "sub"}[e.GetAttributeValue("vlvScope")]
			searches[strings.ToLower(e.DN)] = &VLVIndex{Name: e.GetAttributeValue("cn"), Base: e.GetAttributeValue("vlvBase"),
				Scope: scope, Filter: e.GetAttributeValue("vlvFilter"), Sorts: []string{}, IndexNames: []string{}}
			order = append(order, strings.ToLower(e.DN))
		}
	}
	for _, e := range res.Entries {
		if v := e.GetAttributeValue("vlvSort"); v != "" {
			parent := strings.ToLower(strings.SplitN(e.DN, ",", 2)[1])
			if s := searches[parent]; s != nil {
				s.Sorts = append(s.Sorts, v)
				s.IndexNames = append(s.IndexNames, e.GetAttributeValue("cn"))
			}
		}
	}
	out := make([]VLVIndex, 0, len(order))
	for _, k := range order {
		out = append(out, *searches[k])
	}
	return out, nil
}

// ServerDirs returns the backup and LDIF directories from cn=config.
func ServerDirs(c *ldap.Conn) (bakDir, ldifDir string, err error) {
	e, err := readEntry(c, "cn=config", "nsslapd-bakdir", "nsslapd-ldifdir")
	if err != nil {
		return "", "", err
	}
	return e.GetAttributeValue("nsslapd-bakdir"), e.GetAttributeValue("nsslapd-ldifdir"), nil
}

var (
	vlvNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]*$`)
	vlvScopes = map[string]string{"base": "0", "one": "1", "sub": "2"}
)

// validSort checks a vlvSort value: attributes separated by spaces, each
// optionally prefixed with "-" for descending order.
func validSort(s string) bool {
	f := strings.Fields(s)
	if len(f) == 0 {
		return false
	}
	for _, a := range f {
		if !attrRE.MatchString(strings.TrimPrefix(a, "-")) {
			return false
		}
	}
	return true
}

func vlvSearchDN(backend, name string) string {
	return "cn=" + ldap.EscapeDN(name) + "," + backendDN(backend)
}

// CreateVLV adds a VLV search with one sort index per value of Sorts. The
// indexes are empty until built with a reindex task.
func CreateVLV(c *ldap.Conn, backend string, v VLVIndex) error {
	v.Name = strings.TrimSpace(v.Name)
	if !vlvNameRE.MatchString(v.Name) {
		return fmt.Errorf("%w: VLV name %q", ErrInvalidName, v.Name)
	}
	if _, err := ldap.ParseDN(v.Base); err != nil || strings.TrimSpace(v.Base) == "" {
		return fmt.Errorf("%w: base %q is not a DN", ErrInvalidValue, v.Base)
	}
	scope, ok := vlvScopes[v.Scope]
	if !ok {
		return fmt.Errorf("%w: scope %q", ErrInvalidValue, v.Scope)
	}
	if _, err := ldap.CompileFilter(v.Filter); err != nil {
		return fmt.Errorf("%w: filter: %v", ErrInvalidValue, err)
	}
	if len(v.Sorts) == 0 {
		return fmt.Errorf("%w: at least one sort order is needed", ErrInvalidValue)
	}
	for _, s := range v.Sorts {
		if !validSort(s) {
			return fmt.Errorf("%w: sort order %q", ErrInvalidValue, s)
		}
	}
	if _, err := readEntry(c, backendDN(backend), "1.1"); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			return fmt.Errorf("%w: backend %s", ErrNotFound, backend)
		}
		return err
	}
	dn := vlvSearchDN(backend, v.Name)
	req := ldap.NewAddRequest(dn, nil)
	req.Attribute("objectClass", []string{"top", "vlvSearch"})
	req.Attribute("cn", []string{v.Name})
	req.Attribute("vlvBase", []string{v.Base})
	req.Attribute("vlvScope", []string{scope})
	req.Attribute("vlvFilter", []string{v.Filter})
	if err := c.Add(req); err != nil {
		return err
	}
	for i, s := range v.Sorts {
		name := fmt.Sprintf("%s sort %d", v.Name, i+1)
		ix := ldap.NewAddRequest("cn="+ldap.EscapeDN(name)+","+dn, nil)
		ix.Attribute("objectClass", []string{"top", "vlvIndex"})
		ix.Attribute("cn", []string{name})
		ix.Attribute("vlvSort", []string{strings.Join(strings.Fields(s), " ")})
		if err := c.Add(ix); err != nil {
			_ = deleteVLVEntries(c, dn)
			return err
		}
	}
	return nil
}

// DeleteVLV removes a VLV search and its sort indexes.
func DeleteVLV(c *ldap.Conn, backend, name string) error {
	all, err := ListVLV(c, backend)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(all, func(x VLVIndex) bool { return strings.EqualFold(x.Name, name) }) {
		return fmt.Errorf("%w: VLV index %s", ErrNotFound, name)
	}
	return deleteVLVEntries(c, vlvSearchDN(backend, name))
}

func deleteVLVEntries(c *ldap.Conn, dn string) error {
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=*)", []string{"1.1"}, nil))
	if err != nil && !ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return err
	}
	if res != nil {
		for _, e := range res.Entries {
			if err := c.Del(ldap.NewDelRequest(e.DN, nil)); err != nil {
				return err
			}
		}
	}
	return c.Del(ldap.NewDelRequest(dn, nil))
}
