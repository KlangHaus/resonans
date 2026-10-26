package app

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/KlangHaus/resonans/internal/persistence"
)

// Every branch of the role decision, with what each one logs. The end-to-end
// cell (TestRunRefusesARoleThatBypassesRLSInProduction) proves Run calls this
// with the real role; this one pins the decision itself.
func TestCheckDBRole(t *testing.T) {
	readErr := errors.New("permission denied for view pg_roles")
	for _, c := range []struct {
		name        string
		production  bool
		bypasses    bool
		readErr     error
		wantErr     error
		wantErrText string
		wantLog     string
	}{
		{name: "production, bypassing role", production: true, bypasses: true, wantErr: persistence.ErrRoleBypassesRLS, wantErrText: `"postgres"`},
		{name: "production, unreadable role", production: true, readErr: readErr, wantErrText: "check the database role"},
		{name: "production, app role", production: true, wantLog: `msg=db_role role=postgres bypassesRLS=false`},
		{name: "dev, bypassing role", bypasses: true, wantLog: `level=WARN msg=db_role_bypasses_rls role=postgres`},
		{name: "dev, unreadable role", readErr: readErr, wantLog: `level=WARN msg=db_role_unchecked`},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))
			err := checkDBRole(logger, c.production, "postgres", c.bypasses, c.readErr)
			switch {
			case c.wantErr == nil && c.wantErrText == "" && err != nil:
				t.Fatalf("refused: %v", err)
			case c.wantErr != nil && !errors.Is(err, c.wantErr):
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			case c.wantErrText != "" && (err == nil || !strings.Contains(err.Error(), c.wantErrText)):
				t.Fatalf("err = %v, want it to mention %s", err, c.wantErrText)
			}
			if c.wantLog != "" && !strings.Contains(buf.String(), c.wantLog) {
				t.Errorf("log = %q, want %q", buf.String(), c.wantLog)
			}
			if err != nil && buf.Len() != 0 {
				t.Errorf("a refusal also logged %q; the error is the report", buf.String())
			}
		})
	}
}
