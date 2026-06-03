// Package auth carries the authenticated caller's identity through the
// request lifecycle. Everything past requireAPIKey middleware can pull
// the AuthContext off the request via auth.FromContext.
package auth

import "context"

type Context struct {
	OrganizationID   string
	ProjectID        string
	EnvironmentID    string
	EnvironmentSlug  string
	BetterAuthKeyID  string
	APIKeyMetadataID string
	Scopes           []string
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
