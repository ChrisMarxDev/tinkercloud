package persistence

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/analytics"
	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
)

func TestInsightsAggregateDistinctMarkersRetentionAndDeletion(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	marker := sha256.Sum256([]byte("one"))
	for _, e := range []analytics.Event{{AppID: "a", Marker: marker, HasMarker: true, Occurred: now.AddDate(0, 0, -1)}, {AppID: "a", Marker: marker, HasMarker: true, Occurred: now}, {AppID: "a", Occurred: now}} {
		if err := s.RecordInsight(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := s.InsightSummary(context.Background(), "a", 7, now)
	if err != nil || summary.PageViews != 3 || summary.ApproximateVisitors != 1 || len(summary.Days) != 7 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	if summary.Days[0].Day.Format("2006-01-02") != "2026-07-26" || summary.Days[5].ApproximateVisitors != 1 || summary.Days[6].PageViews != 2 {
		t.Fatalf("wrong zero-filled daily series: %+v", summary.Days)
	}
	gate := &insightsGateSpy{}
	if err := (InsightsControl{Store: s, Recorder: gate}).SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if gate.enabled {
		t.Fatal("local recorder cache not disabled")
	}
	if enabled, err := s.InsightsEnabled(context.Background()); err != nil || enabled {
		t.Fatalf("enabled=%v err=%v", enabled, err)
	}
	if err := s.RecordInsight(context.Background(), analytics.Event{AppID: "a", Occurred: now}); err != nil {
		t.Fatal(err)
	}
	var afterDisable int
	if err := s.DB.QueryRow("SELECT SUM(page_views) FROM app_insight_days WHERE app_id='a'").Scan(&afterDisable); err != nil || afterDisable != 3 {
		t.Fatalf("disabled event persisted: %d %v", afterDisable, err)
	}
	if _, err := s.DB.Exec("INSERT INTO app_insight_days(app_id,day,page_views,last_activity_at) VALUES('a','2026-06-01',9,'2026-06-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupInsights(context.Background(), now, 1); err != nil {
		t.Fatal(err)
	}
	var old int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM app_insight_days WHERE day<'2026-07-02'").Scan(&old); err != nil || old != 0 {
		t.Fatalf("old rows=%d err=%v", old, err)
	}
	if err := (ControlService{Store: s, AppDataCleanup: &lifecyclePurgeSpy{}}).DeleteApp(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", "insights-delete"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"app_insight_days", "app_insight_visitors"} {
		var n int
		if err := s.DB.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE app_id='a'").Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s=%d err=%v", table, n, err)
		}
	}
}

type insightsGateSpy struct{ enabled bool }

func (g *insightsGateSpy) SetEnabled(enabled bool) { g.enabled = enabled }
