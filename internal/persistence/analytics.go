package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/analytics"
)

const insightRetentionDays = 30

type InsightDay struct {
	Day                 time.Time
	PageViews           int64
	ApproximateVisitors int64
}

type InsightSummary struct {
	PageViews           int64
	ApproximateVisitors int64
	LastActivity        time.Time
	Days                []InsightDay
}

// RecordInsight implements analytics.Sink. It is called solely by the
// recorder worker; it never runs in a public request goroutine.
func (s *SQLiteStore) RecordInsight(ctx context.Context, event analytics.Event) error {
	if s == nil || s.DB == nil || event.AppID == "" || event.Occurred.IsZero() {
		return errors.New("insights unavailable")
	}
	when := event.Occurred.UTC()
	day := when.Format("2006-01-02")
	return s.Write(ctx, func(tx *sql.Tx) error {
		var enabled int
		if err := tx.QueryRowContext(ctx, "SELECT enabled FROM local_insights_settings WHERE singleton=1").Scan(&enabled); err != nil || enabled != 1 {
			return nil // Global disable is intentionally a successful drop.
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_insight_days(app_id,day,page_views,last_activity_at) VALUES(?,?,1,?)
			ON CONFLICT(app_id,day) DO UPDATE SET page_views=page_views+1,last_activity_at=CASE WHEN excluded.last_activity_at>last_activity_at THEN excluded.last_activity_at ELSE last_activity_at END`, event.AppID, day, when.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		if event.HasMarker {
			_, err := tx.ExecContext(ctx, `INSERT INTO app_insight_visitors(app_id,day,marker_digest,expires_at) VALUES(?,?,?,?)
				ON CONFLICT(app_id,day,marker_digest) DO UPDATE SET expires_at=excluded.expires_at`, event.AppID, day, event.Marker[:], when.Add(analytics.MarkerLifetime).Format(time.RFC3339Nano))
			return err
		}
		return nil
	})
}

func (s *SQLiteStore) InsightsEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.DB == nil {
		return false, errors.New("insights unavailable")
	}
	var enabled int
	if err := s.DB.QueryRowContext(ctx, "SELECT enabled FROM local_insights_settings WHERE singleton=1").Scan(&enabled); err != nil {
		return false, err
	}
	return enabled == 1, nil
}

// SetInsightsEnabled is the narrow operator-control persistence seam. HTTP
// control/UI wiring intentionally remains outside this backend slice.
func (s *SQLiteStore) SetInsightsEnabled(ctx context.Context, enabled bool) error {
	if s == nil || s.DB == nil {
		return errors.New("insights unavailable")
	}
	value := 0
	if enabled {
		value = 1
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE local_insights_settings SET enabled=?,updated_at=? WHERE singleton=1", value, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

// InsightsControl keeps an operator-triggered durable setting and its
// process-local request-path cache in sync. A future operator command or
// control route must use this seam rather than updating SQLite directly.
type InsightsControl struct {
	Store    *SQLiteStore
	Recorder interface{ SetEnabled(bool) }
}

func (c InsightsControl) SetEnabled(ctx context.Context, enabled bool) error {
	if c.Store == nil || c.Recorder == nil {
		return errors.New("insights control unavailable")
	}
	if err := c.Store.SetInsightsEnabled(ctx, enabled); err != nil {
		return err
	}
	c.Recorder.SetEnabled(enabled)
	return nil
}

// CleanupInsights removes only expired retention data in bounded pages. The
// caller runs it before readiness and from the single maintenance scheduler.
func (s *SQLiteStore) CleanupInsights(ctx context.Context, now time.Time, pageSize int) error {
	if s == nil || s.DB == nil || pageSize < 1 || pageSize > 1000 {
		return errors.New("insights cleanup unavailable")
	}
	// The retained period is exactly 30 UTC calendar dates including today.
	cutoff := utcWindowStart(now, insightRetentionDays).Format("2006-01-02")
	for _, query := range []struct {
		statement string
		value     string
	}{
		{"DELETE FROM app_insight_days WHERE rowid IN (SELECT rowid FROM app_insight_days WHERE day < ? LIMIT ?)", cutoff},
		{"DELETE FROM app_insight_visitors WHERE rowid IN (SELECT rowid FROM app_insight_visitors WHERE day < ? LIMIT ?)", cutoff},
	} {
		for {
			result, err := s.DB.ExecContext(ctx, query.statement, query.value, pageSize)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil || n < int64(pageSize) {
				break
			}
		}
	}
	return nil
}

// InsightSummary uses a single period-wide distinct digest count; daily
// distinct totals are deliberately never summed.
func (s *SQLiteStore) InsightSummary(ctx context.Context, appID string, days int, now time.Time) (InsightSummary, error) {
	if s == nil || s.DB == nil || appID == "" || (days != 7 && days != 30) {
		return InsightSummary{}, errors.New("insights unavailable")
	}
	cutoffTime := utcWindowStart(now, days)
	cutoff := cutoffTime.Format("2006-01-02")
	var out InsightSummary
	var last sql.NullString
	if err := s.DB.QueryRowContext(ctx, "SELECT COALESCE(SUM(page_views),0),MAX(last_activity_at) FROM app_insight_days WHERE app_id=? AND day>=?", appID, cutoff).Scan(&out.PageViews, &last); err != nil {
		return out, err
	}
	if last.Valid {
		out.LastActivity, _ = time.Parse(time.RFC3339Nano, last.String)
	}
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(DISTINCT marker_digest) FROM app_insight_visitors WHERE app_id=? AND day>=?", appID, cutoff).Scan(&out.ApproximateVisitors); err != nil {
		return out, err
	}
	for i := 0; i < days; i++ {
		out.Days = append(out.Days, InsightDay{Day: cutoffTime.AddDate(0, 0, i)})
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT d.day,d.page_views,COUNT(v.marker_digest) FROM app_insight_days d
		LEFT JOIN app_insight_visitors v ON v.app_id=d.app_id AND v.day=d.day
		WHERE d.app_id=? AND d.day>=? GROUP BY d.day,d.page_views ORDER BY d.day`, appID, cutoff)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var day string
		var item InsightDay
		if err := rows.Scan(&day, &item.PageViews, &item.ApproximateVisitors); err != nil {
			return out, err
		}
		item.Day, _ = time.Parse("2006-01-02", day)
		index := int(item.Day.Sub(cutoffTime).Hours() / 24)
		if index >= 0 && index < len(out.Days) {
			out.Days[index].PageViews = item.PageViews
			out.Days[index].ApproximateVisitors = item.ApproximateVisitors
		}
	}
	return out, rows.Err()
}

func utcWindowStart(now time.Time, days int) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -days+1)
}
