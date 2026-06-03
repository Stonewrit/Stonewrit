// Package auth carries the authenticated (or default) caller's scope through
// the request lifecycle. Handlers pull it off the request with auth.FromContext.
package auth

import "context"

type Context struct {
	OrganizationID string
	ProjectID      string
	EnvironmentID  string
	APIKeyID       string
	Scopes         []string
}

// Fixed identifiers for the default tenant used when authentication is
// disabled. The server ensures the project and environment rows exist on boot.
const (
	DefaultOrganizationID = "default"
	DefaultProjectID      = "00000000-0000-0000-0000-000000000001"
	DefaultEnvironmentID  = "00000000-0000-0000-0000-000000000002"
	DefaultAPIKeyID       = "00000000-0000-0000-0000-000000000003"
)

// AllScopes is the full scope set. The default context holds all of them, and
// the CLI grants them to a new key unless told otherwise.
var AllScopes = []string{"events:write", "events:read", "chains:verify", "evidence:read"}

// DefaultContext returns the scope used when authentication is disabled: the
// single built-in default tenant, with every scope.
func DefaultContext() *Context {
	return &Context{
		OrganizationID: DefaultOrganizationID,
		ProjectID:      DefaultProjectID,
		EnvironmentID:  DefaultEnvironmentID,
		APIKeyID:       DefaultAPIKeyID,
		Scopes:         AllScopes,
	}
}

type ctxKey int

const authKey ctxKey = iota

func WithContext(ctx context.Context, ac *Context) context.Context {
	return context.WithValue(ctx, authKey, ac)
}

func FromContext(ctx context.Context) (*Context, bool) {
	ac, ok := ctx.Value(authKey).(*Context)
	return ac, ok
}

func (c *Context) HasScope(required string) bool {
	for _, s := range c.Scopes {
		if s == required {
			return true
		}
	}
	return false
}
