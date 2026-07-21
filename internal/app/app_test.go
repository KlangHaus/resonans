package app

import (
	"context"
	"testing"
)

func TestMigrateRequiresDatabaseURL(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("DATABASE_URL", "")
	if err := Migrate(context.Background()); err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}
