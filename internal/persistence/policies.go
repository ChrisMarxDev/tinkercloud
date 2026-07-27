package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/tinyhost/tiny/internal/policies"
)

func (s *SQLiteStore) Current(ctx context.Context, appID string) (policies.Policy, error) {
	var p policies.Policy
	var mode string
	e := s.DB.QueryRowContext(ctx, "SELECT a.id,a.policy_revision,p.mode,u.normalized_email FROM applications a JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision JOIN users u ON u.id=a.owner_user_id WHERE a.id=? AND a.status='active'", appID).Scan(&p.AppID, &p.Revision, &mode, &p.OwnerIdentityID)
	if e != nil || mode != "private" {
		return policies.Policy{}, policies.ErrUnavailable
	}
	p.Valid = true
	p.Emails = map[string]struct{}{}
	p.Domains = map[string]struct{}{}
	rows, e := s.DB.QueryContext(ctx, "SELECT kind,normalized_value FROM access_rules WHERE app_id=? AND policy_revision=?", appID, p.Revision)
	if e != nil {
		return policies.Policy{}, policies.ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if e = rows.Scan(&k, &v); e != nil {
			return policies.Policy{}, policies.ErrUnavailable
		}
		switch k {
		case "email":
			p.Emails[v] = struct{}{}
		case "domain":
			p.Domains[v] = struct{}{}
		case "owner":
		default:
			return policies.Policy{}, policies.ErrUnavailable
		}
	}
	if rows.Err() != nil {
		return policies.Policy{}, policies.ErrUnavailable
	}
	return p, nil
}
func (s *SQLiteStore) ReplacePolicy(ctx context.Context, actor, appID string, emails, domains []string, requestID string) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		var rev uint64
		var owner, status string
		if e := tx.QueryRowContext(ctx, "SELECT policy_revision,owner_user_id,status FROM applications WHERE id=?", appID).Scan(&rev, &owner, &status); e != nil || owner != actor || status != "active" {
			return fmt.Errorf("policy denied")
		}
		next := rev + 1
		if _, e := tx.ExecContext(ctx, "INSERT INTO access_policies(app_id,revision,mode,created_by,created_at) VALUES(?,?, 'private', ?, datetime('now'))", appID, next, actor); e != nil {
			return e
		}
		for _, v := range emails {
			if _, e := tx.ExecContext(ctx, "INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES(lower(hex(randomblob(16))),?,?, 'email', ?, ?, datetime('now'))", appID, next, v, actor); e != nil {
				return e
			}
		}
		for _, v := range domains {
			if _, e := tx.ExecContext(ctx, "INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES(lower(hex(randomblob(16))),?,?, 'domain', ?, ?, datetime('now'))", appID, next, v, actor); e != nil {
				return e
			}
		}
		if _, e := tx.ExecContext(ctx, "UPDATE applications SET policy_revision=?,updated_at=datetime('now') WHERE id=?", next, appID); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,app_id,action,outcome,request_id) VALUES(lower(hex(randomblob(16))),datetime('now'),'user',?,?, 'policy.replace','success',?)", actor, appID, requestID)
		return e
	})
}
