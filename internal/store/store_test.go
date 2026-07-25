package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// makeDB creates a session-store-like DB with the given rows.
// Each row is {session_id, model, total_nano_aiu, created_at}.
func makeDB(t *testing.T, rows [][4]any) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session-store.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE assistant_usage_events (
		id INTEGER PRIMARY KEY,
		session_id TEXT, turn_index INTEGER, agent_id TEXT, model TEXT,
		total_nano_aiu INTEGER, created_at TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO assistant_usage_events(session_id, model, total_nano_aiu, created_at)
			 VALUES(?,?,?,?)`, r[0], r[1], r[2], r[3]); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return path
}

func find(u *Usage, key string) *Series {
	for i := range u.Series {
		if u.Series[i].Key == key {
			return &u.Series[i]
		}
	}
	return nil
}

func TestAggregateByModelDaily(t *testing.T) {
	// 1e9 nano = 1 AIU.
	path := makeDB(t, [][4]any{
		{"s1", "gpt-5-mini", int64(500_000_000), "2026-07-25T01:00:00.000Z"}, // 0.5 AIU
		{"s1", "gpt-5-mini", int64(500_000_000), "2026-07-25T05:00:00.000Z"}, // 0.5 AIU same day
		{"s2", "claude", int64(2_000_000_000), "2026-07-26T02:00:00.000Z"},   // 2 AIU next day
	})
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	u, err := st.Aggregate(context.Background(), DimModel, GranDay)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(u.Buckets) != 2 {
		t.Fatalf("buckets = %v, want 2", u.Buckets)
	}
	if u.Buckets[0] != "2026-07-25" || u.Buckets[1] != "2026-07-26" {
		t.Fatalf("bucket labels = %v", u.Buckets)
	}
	// Ordered by total desc: claude (2.0) before gpt-5-mini (1.0).
	if u.Series[0].Key != "claude" {
		t.Fatalf("first series = %q, want claude", u.Series[0].Key)
	}
	gpt := find(u, "gpt-5-mini")
	if gpt == nil || gpt.Values[0] != 1.0 || gpt.Values[1] != 0.0 {
		t.Fatalf("gpt-5-mini values = %+v, want [1 0]", gpt)
	}
	cl := find(u, "claude")
	if cl == nil || cl.Values[0] != 0.0 || cl.Values[1] != 2.0 {
		t.Fatalf("claude values = %+v, want [0 2]", cl)
	}
	if u.TotalAIU != 3.0 {
		t.Fatalf("total = %v, want 3.0", u.TotalAIU)
	}
	if u.Rows != 3 {
		t.Fatalf("rows = %d, want 3", u.Rows)
	}
}

func TestAggregateBySessionMonthly(t *testing.T) {
	path := makeDB(t, [][4]any{
		{"s1", "m", int64(1_000_000_000), "2026-07-01T00:00:00.000Z"},
		{"s2", "m", int64(1_000_000_000), "2026-07-31T23:00:00.000Z"},
		{"s1", "m", int64(1_000_000_000), "2026-08-02T00:00:00.000Z"},
	})
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	u, err := st.Aggregate(context.Background(), DimSession, GranMonth)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(u.Buckets) != 2 || u.Buckets[0] != "2026-07" || u.Buckets[1] != "2026-08" {
		t.Fatalf("buckets = %v", u.Buckets)
	}
	s1 := find(u, "s1")
	if s1 == nil || s1.Values[0] != 1.0 || s1.Values[1] != 1.0 {
		t.Fatalf("s1 values = %+v, want [1 1]", s1)
	}
	s2 := find(u, "s2")
	if s2 == nil || s2.Values[0] != 1.0 || s2.Values[1] != 0.0 {
		t.Fatalf("s2 values = %+v, want [1 0]", s2)
	}
}

func TestUnknownSeriesLabel(t *testing.T) {
	path := makeDB(t, [][4]any{
		{"s1", nil, int64(1_000_000_000), "2026-07-25T01:00:00.000Z"},
	})
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	u, err := st.Aggregate(context.Background(), DimModel, GranDay)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if find(u, "unknown") == nil {
		t.Fatalf("expected 'unknown' series, got %+v", u.Series)
	}
}
