package persistence

import (
	"context"
	"github.com/tinyhost/tiny/internal/controlapi"
	"testing"
)

func TestControlAccessOwnedSorted(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	now := "datetime('now')"
	_, _ = s.DB.Exec("INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private'," + now + ")")
	_, _ = s.DB.Exec("INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_at) VALUES('e','a',1,'email','z@example.com'," + now + "),('d','a',1,'domain','example.com'," + now + ")")
	v, e := (ControlService{Store: s}).Access(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha")
	if e != nil {
		t.Fatal(e)
	}
	x := v.(AccessView)
	if x.Mode != "private" || len(x.Allow.Emails) != 1 || x.Allow.Emails[0] != "z@example.com" {
		t.Fatal(x)
	}
	if _, e = (ControlService{Store: s}).Access(context.Background(), controlapi.Actor{ID: "other", Active: true}, "alpha"); e == nil {
		t.Fatal("cross owner")
	}
}
