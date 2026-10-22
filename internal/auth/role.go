package auth

import (
	"fmt"
	"net/http"
	"strings"
)

// Role is what a verified actor may do in its tenant (ADR 0001 §3). The four
// roles are ordered: each one includes everything the one below it can do.
//
// RoleNone is the zero value on purpose. An Actor built without a role — a
// token whose role claim is missing or unrecognized, or a code path that forgot
// to set one — can do nothing, rather than inheriting a default.
type Role int

const (
	RoleNone Role = iota
	RoleViewer
	RoleEditor
	RolePublisher
	RoleAdmin
)

var roleNames = map[Role]string{
	RoleNone:      "none",
	RoleViewer:    "viewer",
	RoleEditor:    "editor",
	RolePublisher: "publisher",
	RoleAdmin:     "admin",
}

func (r Role) String() string {
	if n, ok := roleNames[r]; ok {
		return n
	}
	return fmt.Sprintf("Role(%d)", int(r))
}

// AtLeast reports whether r includes min. RoleNone includes nothing, not even
// RoleNone: a route that asks for RoleNone is a route nobody can reach.
func (r Role) AtLeast(minRole Role) bool {
	return r != RoleNone && minRole != RoleNone && r >= minRole
}

// ParseRole maps a role name to its Role. Only the four names are accepted;
// "none" is not a role a token can claim.
func ParseRole(s string) (Role, bool) {
	switch strings.TrimSpace(s) {
	case "viewer":
		return RoleViewer, true
	case "editor":
		return RoleEditor, true
	case "publisher":
		return RolePublisher, true
	case "admin":
		return RoleAdmin, true
	}
	return RoleNone, false
}

// ParseRoleMap reads RESONANS_ROLE_MAP: comma-separated `provider-value=role`
// pairs, e.g. "cms-admins=admin,owner=admin,member=editor". It lets a
// self-hoster use the groups or organization roles their provider already
// issues. An unknown role name, an empty side or a duplicate key is an error —
// a typo here would otherwise quietly leave every editor locked out, or worse,
// map a group to a role nobody intended.
func ParseRoleMap(s string) (map[string]Role, error) {
	m := map[string]Role{}
	if strings.TrimSpace(s) == "" {
		return m, nil
	}
	for _, pair := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(pair, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok || k == "" || v == "" {
			return nil, fmt.Errorf("role map: %q is not provider-value=role", strings.TrimSpace(pair))
		}
		role, known := ParseRole(v)
		if !known {
			return nil, fmt.Errorf("role map: %q maps to unknown role %q (want viewer|editor|publisher|admin)", k, v)
		}
		if _, dup := m[k]; dup {
			return nil, fmt.Errorf("role map: %q is mapped twice", k)
		}
		m[k] = role
	}
	return m, nil
}

// RequireRole wraps h so it runs only for an actor with at least minRole.
// No actor, or too low a role, is 403: the request was authenticated (or the
// auth middleware would already have said 401), it is just not allowed.
func RequireRole(minRole Role, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := ActorFromContext(r.Context())
		if !ok || !actor.Role.AtLeast(minRole) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"requires role ` + minRole.String() + `"}`))
			return
		}
		h(w, r)
	}
}
