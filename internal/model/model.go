// Package model holds the documentation model that gets embedded into the
// generated HTML. The JSON shape is part of the tool's contract: the template
// reads it directly, so field names must not change without updating the
// template as well.
package model

import (
	"encoding/json"
	"strings"
)

// Access levels. A protected endpoint is either public, authenticated (any
// logged-in user), denied, or restricted to a set of roles.
const (
	AccessPublic        = "public"
	AccessAuthenticated = "authenticated"
	AccessDenied        = "denied"
	AccessRoles         = "roles"
)

// Access is either a named level or a role list. It marshals to a bare string
// for levels and to a string array for roles, which is what the template
// expects.
type Access struct {
	Kind  string
	Roles []string
}

// Public returns the public access level.
func Public() Access { return Access{Kind: AccessPublic} }

// Authenticated returns the authenticated access level.
func Authenticated() Access { return Access{Kind: AccessAuthenticated} }

// Denied returns the denied access level.
func Denied() Access { return Access{Kind: AccessDenied} }

// Roles returns a role-restricted access level.
func Roles(roles []string) Access { return Access{Kind: AccessRoles, Roles: roles} }

// IsRoles reports whether the access level is a role list.
func (a Access) IsRoles() bool { return a.Kind == AccessRoles }

// MarshalJSON encodes levels as strings and roles as arrays.
func (a Access) MarshalJSON() ([]byte, error) {
	if a.IsRoles() {
		if a.Roles == nil {
			return []byte("[]"), nil
		}
		return json.Marshal(a.Roles)
	}
	return json.Marshal(a.Kind)
}

// UnmarshalJSON accepts either a string level or a role array.
func (a *Access) UnmarshalJSON(data []byte) error {
	var level string
	if err := json.Unmarshal(data, &level); err == nil {
		a.Kind = level
		a.Roles = nil
		return nil
	}
	var roles []string
	if err := json.Unmarshal(data, &roles); err != nil {
		return err
	}
	a.Kind = AccessRoles
	a.Roles = roles
	return nil
}

// String renders the access for terminal output.
func (a Access) String() string {
	if a.IsRoles() {
		if len(a.Roles) == 0 {
			return "roles()"
		}
		return strings.Join(a.Roles, ", ")
	}
	return a.Kind
}

// Equal reports whether two access values describe the same restriction.
func (a Access) Equal(other Access) bool {
	if a.Kind != other.Kind {
		return false
	}
	if len(a.Roles) != len(other.Roles) {
		return false
	}
	for i := range a.Roles {
		if a.Roles[i] != other.Roles[i] {
			return false
		}
	}
	return true
}

// Param is a single endpoint parameter.
type Param struct {
	In       string `json:"in"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Desc     string `json:"desc"`
}

// Response is one documented status code.
type Response struct {
	Status string `json:"status"`
	Label  string `json:"label"`
	Desc   string `json:"desc"`
	JSON   any    `json:"json"`
}

// Endpoint is an endpoint extracted from source.
type Endpoint struct {
	Method    string     `json:"method"`
	Path      string     `json:"path"`
	Title     string     `json:"title"`
	Desc      string     `json:"desc"`
	Access    Access     `json:"access"`
	Params    []Param    `json:"params"`
	Body      any        `json:"body"`
	Returns   string     `json:"returns"`
	Responses []Response `json:"responses"`
	Notes     []string   `json:"notes"`
	Errors    []string   `json:"errors"`
}

// Group is a named set of endpoints.
//
// Endpoints is []any on purpose: extracted endpoints are typed, while curated
// ones are passed through as decoded JSON so hand-written examples and extra
// keys survive untouched.
type Group struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Endpoints   []any  `json:"endpoints"`
}

// Meta is the document header.
type Meta struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	BaseURL  string `json:"baseUrl"`
}

// ErrorEntry is one entry of the error catalogue.
type ErrorEntry struct {
	Code string `json:"code"`
	Desc string `json:"desc"`
	JSON any    `json:"json"`
}

// Data is the full document model.
type Data struct {
	Meta        Meta    `json:"meta"`
	AuthNote    string  `json:"authNote"`
	Conventions any     `json:"conventions"`
	Changes     any     `json:"changes"`
	Enums       any     `json:"enums"`
	Errors      any     `json:"errors"`
	Models      any     `json:"models"`
	Groups      []Group `json:"groups"`
}

// Total counts every documented endpoint.
func (d Data) Total() int {
	total := 0
	for _, g := range d.Groups {
		total += len(g.Endpoints)
	}
	return total
}
