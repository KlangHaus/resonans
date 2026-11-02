package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrRoleBypassesRLS is the refusal to serve as a role that row level security
// does not apply to.
//
// The stores' queries carry no tenant_id: tenant isolation is the RLS policy
// alone, and Postgres skips every policy for a SUPERUSER or a BYPASSRLS role,
// FORCE notwithstanding. Measured in internal/app's integration cell before
// this check existed: the same server over the same rows let another tenant
// list every document as a superuser, and none as the app role. Nothing said
// so — the server started and served.
var ErrRoleBypassesRLS = errors.New("the database role bypasses row level security (SUPERUSER or BYPASSRLS), so tenant isolation would not apply")

// ConnectedRole reports the role the pool connects as and whether row level
// security applies to it. It reads the role's own attributes: BYPASSRLS and
// SUPERUSER are not inherited through membership, so the connected role is the
// one that decides.
func ConnectedRole(ctx context.Context, pool *pgxpool.Pool) (role string, bypassesRLS bool, err error) {
	err = pool.QueryRow(ctx,
		`select current_user, rolsuper or rolbypassrls from pg_roles where rolname = current_user`,
	).Scan(&role, &bypassesRLS)
	if err != nil {
		return "", false, fmt.Errorf("persistence: read the connected role: %w", err)
	}
	return role, bypassesRLS, nil
}
