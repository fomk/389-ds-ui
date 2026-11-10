package ds

import (
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// PluginTaskParam is an attribute of a plug-in task entry.
type PluginTaskParam struct {
	Attr     string   `json:"attr"`
	Kind     string   `json:"kind" enum:"dn,filter,enum"`
	Required bool     `json:"required,omitempty"`
	Default  string   `json:"default,omitempty"`
	Values   []string `json:"values,omitempty"`
}

// PluginTask is a maintenance task a plug-in offers (cn=<container>,cn=tasks).
type PluginTask struct {
	ID        string            `json:"id"`
	Plugin    string            `json:"plugin"`
	Container string            `json:"-"`
	Params    []PluginTaskParam `json:"params"`
}

// PluginTasks are the plug-in tasks ds-ui can start.
var PluginTasks = []PluginTask{
	{ID: "memberof-fixup", Plugin: "MemberOf Plugin", Container: "memberof task", Params: []PluginTaskParam{
		{Attr: "basedn", Kind: "dn", Required: true},
		{Attr: "filter", Kind: "filter", Default: "(objectClass=*)"},
	}},
	{ID: "automember-rebuild", Plugin: "Auto Membership Plugin", Container: "automember rebuild membership", Params: []PluginTaskParam{
		{Attr: "basedn", Kind: "dn", Required: true},
		{Attr: "filter", Kind: "filter", Required: true, Default: "(objectClass=*)"},
		{Attr: "scope", Kind: "enum", Values: []string{"sub", "one", "base"}, Default: "sub"},
	}},
	{ID: "linkedattrs-fixup", Plugin: "Linked Attributes", Container: "fixup linked attributes", Params: []PluginTaskParam{
		{Attr: "linkdn", Kind: "dn"},
	}},
}

// PluginTaskAttrs validates the parameters of a plug-in task and returns
// the task, its target (for the history) and the entry attributes.
func PluginTaskAttrs(id string, params map[string]string) (*PluginTask, string, map[string][]string, error) {
	var task *PluginTask
	for i := range PluginTasks {
		if PluginTasks[i].ID == id {
			task = &PluginTasks[i]
		}
	}
	if task == nil {
		return nil, "", nil, fmt.Errorf("%w: plug-in task %s", ErrNotFound, id)
	}
	attrs := map[string][]string{}
	target := task.Plugin
	for _, p := range task.Params {
		v := strings.TrimSpace(params[p.Attr])
		if v == "" {
			v = p.Default
		}
		if v == "" {
			if p.Required {
				return nil, "", nil, fmt.Errorf("%w: %s is required", ErrInvalidValue, p.Attr)
			}
			continue
		}
		switch p.Kind {
		case "dn":
			if _, err := ldap.ParseDN(v); err != nil {
				return nil, "", nil, fmt.Errorf("%w: %s is not a DN", ErrInvalidValue, p.Attr)
			}
			if target == task.Plugin {
				target = v
			}
		case "filter":
			if _, err := ldap.CompileFilter(v); err != nil {
				return nil, "", nil, fmt.Errorf("%w: %s: %v", ErrInvalidValue, p.Attr, err)
			}
		case "enum":
			ok := false
			for _, x := range p.Values {
				ok = ok || x == v
			}
			if !ok {
				return nil, "", nil, fmt.Errorf("%w: %s must be one of %s", ErrInvalidValue, p.Attr, strings.Join(p.Values, ", "))
			}
		}
		attrs[p.Attr] = []string{v}
	}
	return task, target, attrs, nil
}
