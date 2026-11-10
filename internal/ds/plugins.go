package ds

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// DNPlugins is the container of all plug-in entries.
const DNPlugins = "cn=plugins,cn=config"

// ErrProtectedPlugin is returned when disabling a plug-in the server cannot
// run without (syntaxes, matching rules, the database, access control).
var ErrProtectedPlugin = errors.New("this plug-in cannot be disabled")

// Plugin is a server plug-in (an nsSlapdPlugin entry).
type Plugin struct {
	Name        string   `json:"name" doc:"cn"`
	DN          string   `json:"dn"`
	Enabled     bool     `json:"enabled"`
	Type        string   `json:"type" doc:"nsslapd-pluginType"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	ID          string   `json:"id,omitempty" doc:"nsslapd-pluginId"`
	Path        string   `json:"path,omitempty"`
	DependsOn   []string `json:"dependsOn" doc:"Named plug-ins and plug-in types this one depends on"`
	Protected   bool     `json:"protected" doc:"The server needs it; it cannot be disabled here"`
}

var pluginAttrs = []string{"cn", "nsslapd-pluginEnabled", "nsslapd-pluginType", "nsslapd-pluginDescription",
	"nsslapd-pluginVersion", "nsslapd-pluginVendor", "nsslapd-pluginId", "nsslapd-pluginPath",
	"nsslapd-plugin-depends-on-named", "nsslapd-plugin-depends-on-type"}

// Plug-in types and names that must stay enabled.
var (
	protectedTypes = []string{"syntax", "matchingrule"}
	protectedNames = []string{"ldbm database", "acl plugin", "acl preoperation"}
)

func pluginFromEntry(e *ldap.Entry) Plugin {
	p := Plugin{
		Name:        e.GetAttributeValue("cn"),
		DN:          e.DN,
		Enabled:     strings.EqualFold(e.GetAttributeValue("nsslapd-pluginEnabled"), "on"),
		Type:        e.GetAttributeValue("nsslapd-pluginType"),
		Description: e.GetAttributeValue("nsslapd-pluginDescription"),
		Version:     e.GetAttributeValue("nsslapd-pluginVersion"),
		Vendor:      e.GetAttributeValue("nsslapd-pluginVendor"),
		ID:          e.GetAttributeValue("nsslapd-pluginId"),
		Path:        e.GetAttributeValue("nsslapd-pluginPath"),
		DependsOn:   []string{},
	}
	p.DependsOn = append(p.DependsOn, e.GetAttributeValues("nsslapd-plugin-depends-on-named")...)
	p.DependsOn = append(p.DependsOn, e.GetAttributeValues("nsslapd-plugin-depends-on-type")...)
	p.Protected = slices.Contains(protectedTypes, strings.ToLower(p.Type)) ||
		slices.Contains(protectedNames, strings.ToLower(p.Name))
	return p
}

// ListPlugins returns all plug-ins, sorted by name.
func ListPlugins(c *ldap.Conn) ([]Plugin, error) {
	res, err := c.Search(ldap.NewSearchRequest(DNPlugins, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, "(objectClass=nsSlapdPlugin)", pluginAttrs, nil))
	if err != nil {
		return nil, err
	}
	out := make([]Plugin, 0, len(res.Entries))
	for _, e := range res.Entries {
		out = append(out, pluginFromEntry(e))
	}
	slices.SortFunc(out, func(a, b Plugin) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

// GetPlugin finds a plug-in by its cn.
func GetPlugin(c *ldap.Conn, name string) (*Plugin, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: empty plug-in name", ErrInvalidName)
	}
	res, err := c.Search(ldap.NewSearchRequest(DNPlugins, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		2, 0, false, fmt.Sprintf("(&(objectClass=nsSlapdPlugin)(cn=%s))", ldap.EscapeFilter(name)), pluginAttrs, nil))
	if err != nil {
		return nil, err
	}
	if len(res.Entries) == 0 {
		return nil, fmt.Errorf("%w: plug-in %q", ErrNotFound, name)
	}
	p := pluginFromEntry(res.Entries[0])
	return &p, nil
}

// DynamicPlugins reports whether plug-in changes apply without a restart.
func DynamicPlugins(c *ldap.Conn) (bool, error) {
	res, err := c.Search(ldap.NewSearchRequest("cn=config", ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{"nsslapd-dynamic-plugins"}, nil))
	if err != nil {
		return false, err
	}
	return len(res.Entries) == 1 && strings.EqualFold(res.Entries[0].GetAttributeValue("nsslapd-dynamic-plugins"), "on"), nil
}

// SetPluginEnabled turns a plug-in on or off. It reports whether the change
// waits for a restart (nsslapd-dynamic-plugins is off).
func SetPluginEnabled(c *ldap.Conn, name string, enabled bool) (restart bool, err error) {
	p, err := GetPlugin(c, name)
	if err != nil {
		return false, err
	}
	if !enabled && p.Protected {
		return false, fmt.Errorf("%w: %s", ErrProtectedPlugin, p.Name)
	}
	if p.Enabled == enabled {
		return false, nil
	}
	req := ldap.NewModifyRequest(p.DN, nil)
	req.Replace("nsslapd-pluginEnabled", []string{onOff(enabled)})
	if err := c.Modify(req); err != nil {
		return false, err
	}
	dynamic, err := DynamicPlugins(c)
	if err != nil {
		return false, err
	}
	return !dynamic, nil
}
