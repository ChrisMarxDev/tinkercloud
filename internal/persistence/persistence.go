// Package persistence reserves the repository boundary for the approved SQLite spike.
package persistence

import (
	"context"
	"database/sql"
)

type Migration struct {
	Version int
	Name    string
}

var Migrations = []Migration{}

// DBTX is the narrow database/sql seam shared by future repositories; callers
// may supply *sql.DB or *sql.Tx without making domain packages driver-aware.
type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
