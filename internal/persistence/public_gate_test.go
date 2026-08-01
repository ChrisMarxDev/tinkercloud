package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/policies"
)

func TestPublicGateMigrationDefaultsOffAtRevisionOne(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	gate, err := s.CurrentPublicGate(context.Background())
	if err != nil || !gate.Valid || gate.Enabled || gate.Revision != 1 {
		t.Fatalf("default gate=%#v err=%v", gate, err)
	}
}

func TestPublicGateRootCASRevisionAndAudit(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	ctx := context.Background()
	if err := s.SetPublicGate(ctx, "root", true, 1, "root-enable"); err != nil {
		t.Fatal(err)
	}
	gate, err := s.CurrentPublicGate(ctx)
	if err != nil || !gate.Enabled || gate.Revision != 2 {
		t.Fatalf("enabled gate=%#v err=%v", gate, err)
	}
	if err := s.SetPublicGate(ctx, "root", false, 1, "stale"); !errors.Is(err, policies.ErrUnavailable) {
		t.Fatalf("stale revision=%v", err)
	}
	if err := s.SetPublicGate(ctx, "op", false, 2, "wrong-actor"); !errors.Is(err, policies.ErrUnavailable) {
		t.Fatalf("non-root actor=%v", err)
	}
	if err := s.SetPublicGate(ctx, "root", false, 2, "root-disable"); err != nil {
		t.Fatal(err)
	}
	gate, err = s.CurrentPublicGate(ctx)
	if err != nil || gate.Enabled || gate.Revision != 3 {
		t.Fatalf("disabled gate=%#v err=%v", gate, err)
	}
	var count int
	var kind string
	var actor any
	if err := s.DB.QueryRow("SELECT COUNT(*),MIN(actor_kind),MIN(actor_id) FROM audit_events WHERE action='public_static_gate.set'").Scan(&count, &kind, &actor); err != nil || count != 2 || kind != "root" || actor != nil {
		t.Fatalf("audit count=%d kind=%q actor=%v err=%v", count, kind, actor, err)
	}
}

func TestPublicGateMissingOrCorruptFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name       string
		breakState func(*testing.T, *SQLiteStore)
	}{
		{name: "missing", breakState: func(t *testing.T, s *SQLiteStore) {
			if _, err := s.DB.Exec("DELETE FROM public_static_settings"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "corrupt", breakState: func(t *testing.T, s *SQLiteStore) {
			if _, err := s.DB.Exec("PRAGMA ignore_check_constraints=ON; UPDATE public_static_settings SET enabled=2"); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := seeded(t)
			defer s.Close()
			tc.breakState(t, s)
			if gate, err := s.CurrentPublicGate(context.Background()); !errors.Is(err, policies.ErrUnavailable) || gate.Valid {
				t.Fatalf("gate=%#v err=%v", gate, err)
			}
			if err := s.SetPublicGate(context.Background(), "root", true, 1, "denied"); !errors.Is(err, policies.ErrUnavailable) {
				t.Fatalf("mutation=%v", err)
			}
		})
	}
}

func TestPublicGateAuditFailureRollsBackSetting(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	if _, err := s.DB.Exec(`CREATE TRIGGER deny_public_gate_audit BEFORE INSERT ON audit_events
		WHEN NEW.action='public_static_gate.set' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPublicGate(context.Background(), "root", true, 1, "audit-fails"); err == nil {
		t.Fatal("audit failure accepted")
	}
	gate, err := s.CurrentPublicGate(context.Background())
	if err != nil || gate.Enabled || gate.Revision != 1 {
		t.Fatalf("rolled-back gate=%#v err=%v", gate, err)
	}
}
