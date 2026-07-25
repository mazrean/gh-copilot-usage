package store

import (
	"context"
	"database/sql"
	"errors"
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
	if _, err := db.Exec(`CREATE TABLE sessions (
		id TEXT PRIMARY KEY, cwd TEXT, repository TEXT, branch TEXT,
		summary TEXT, created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE checkpoints (
		id INTEGER PRIMARY KEY, session_id TEXT, checkpoint_number INTEGER,
		title TEXT, overview TEXT, work_done TEXT, next_steps TEXT, created_at TEXT)`); err != nil {
		t.Fatalf("create checkpoints: %v", err)
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

// insertSession adds a sessions row. Empty strings are stored as NULL.
func insertSession(t *testing.T, path, id, summary, repository, branch string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	toNull := func(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
	if _, err := db.Exec(
		`INSERT INTO sessions(id, summary, repository, branch) VALUES(?,?,?,?)`,
		id, toNull(summary), toNull(repository), toNull(branch)); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

// insertCheckpoint adds a checkpoints row for a session.
func insertCheckpoint(t *testing.T, path, sessionID string, number int, title, overview, workDone, nextSteps string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(
		`INSERT INTO checkpoints(session_id, checkpoint_number, title, overview, work_done, next_steps, created_at)
		 VALUES(?,?,?,?,?,?,?)`,
		sessionID, number, title, overview, workDone, nextSteps, "2026-07-25T01:00:00.000Z"); err != nil {
		t.Fatalf("insert checkpoint: %v", err)
	}
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

func TestSessionLabelFromSummary(t *testing.T) {
	path := makeDB(t, [][4]any{
		{"s1", "m", int64(1_000_000_000), "2026-07-25T01:00:00.000Z"},
	})
	insertSession(t, path, "s1", "Fix login bug", "", "")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	u, err := st.Aggregate(context.Background(), DimSession, GranDay)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	s1 := find(u, "s1")
	if s1 == nil || s1.Label != "Fix login bug" {
		t.Fatalf("label = %+v, want %q", s1, "Fix login bug")
	}
}

func TestSessionLabelFallsBackToRepository(t *testing.T) {
	path := makeDB(t, [][4]any{
		{"s1", "m", int64(1_000_000_000), "2026-07-25T01:00:00.000Z"},
	})
	insertSession(t, path, "s1", "", "example-org/example-repo", "main")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	u, err := st.Aggregate(context.Background(), DimSession, GranDay)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	s1 := find(u, "s1")
	if s1 == nil || s1.Label != "example-org/example-repo (main)" {
		t.Fatalf("label = %+v, want %q", s1, "example-org/example-repo (main)")
	}
}

func TestSessionLabelFallsBackToKeyWhenNoMetadata(t *testing.T) {
	path := makeDB(t, [][4]any{
		{"s1", "m", int64(1_000_000_000), "2026-07-25T01:00:00.000Z"},
	})

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	u, err := st.Aggregate(context.Background(), DimSession, GranDay)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	s1 := find(u, "s1")
	if s1 == nil || s1.Label != "s1" {
		t.Fatalf("label = %+v, want %q", s1, "s1")
	}
}

func TestSessionDetail(t *testing.T) {
	path := makeDB(t, [][4]any{
		{"s1", "gpt-5-mini", int64(500_000_000), "2026-07-25T01:00:00.000Z"},
		{"s1", "claude", int64(1_000_000_000), "2026-07-25T02:00:00.000Z"},
	})
	insertSession(t, path, "s1", "Fix login bug", "example-org/example-repo", "main")
	insertCheckpoint(t, path, "s1", 1, "Initial exploration", "looked around", "read files", "start implementing")
	insertCheckpoint(t, path, "s1", 2, "Implementation", "wrote code", "added feature", "write tests")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	d, err := st.SessionDetail(context.Background(), "s1")
	if err != nil {
		t.Fatalf("session detail: %v", err)
	}
	if d.Summary != "Fix login bug" || d.Repository != "example-org/example-repo" || d.Branch != "main" {
		t.Fatalf("metadata = %+v", d)
	}
	if len(d.ByModel) != 2 || d.ByModel[0].Model != "claude" || d.ByModel[0].AIU != 1.0 {
		t.Fatalf("byModel = %+v, want claude first with 1.0 AIU", d.ByModel)
	}
	if len(d.Checkpoints) != 2 || d.Checkpoints[0].Title != "Initial exploration" || d.Checkpoints[1].Title != "Implementation" {
		t.Fatalf("checkpoints = %+v", d.Checkpoints)
	}
}

func TestSessionDetailNotFound(t *testing.T) {
	path := makeDB(t, [][4]any{
		{"s1", "m", int64(1_000_000_000), "2026-07-25T01:00:00.000Z"},
	})
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	_, err = st.SessionDetail(context.Background(), "does-not-exist")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestModelDetailWithoutBreakdownColumn(t *testing.T) {
	// makeDB's fixture schema has no token_details_json column, matching an
	// older Copilot CLI session-store.db.
	path := makeDB(t, [][4]any{
		{"s1", "gpt-5-mini", int64(500_000_000), "2026-07-25T01:00:00.000Z"},
		{"s2", "gpt-5-mini", int64(300_000_000), "2026-07-25T02:00:00.000Z"},
		{"s1", "claude", int64(1_000_000_000), "2026-07-25T03:00:00.000Z"},
	})
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	d, err := st.ModelDetail(context.Background(), "gpt-5-mini")
	if err != nil {
		t.Fatalf("model detail: %v", err)
	}
	if d.AIU != 0.8 || d.Rows != 2 {
		t.Fatalf("aiu/rows = %v/%d, want 0.8/2", d.AIU, d.Rows)
	}
	if d.ByCategory != nil {
		t.Fatalf("byCategory = %+v, want nil (no breakdown column)", d.ByCategory)
	}
}

func TestModelDetailUnknownModel(t *testing.T) {
	path := makeDB(t, [][4]any{
		{"s1", "gpt-5-mini", int64(500_000_000), "2026-07-25T01:00:00.000Z"},
	})
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	d, err := st.ModelDetail(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("model detail: %v", err)
	}
	if d.AIU != 0 || d.Rows != 0 {
		t.Fatalf("aiu/rows = %v/%d, want 0/0 for an unused model", d.AIU, d.Rows)
	}
}

// makeDBWithBreakdown creates a session-store-like DB whose
// assistant_usage_events table includes token_details_json, matching current
// Copilot CLI schema (schema_version 6). Each row is
// {model, total_nano_aiu, tokenDetailsJSON, created_at}; tokenDetailsJSON may
// be nil to simulate an event predating per-category cost breakdown.
func makeDBWithBreakdown(t *testing.T, rows [][4]any) string {
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
		total_nano_aiu INTEGER, token_details_json TEXT, created_at TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions (
		id TEXT PRIMARY KEY, cwd TEXT, repository TEXT, branch TEXT,
		summary TEXT, created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO assistant_usage_events(session_id, model, total_nano_aiu, token_details_json, created_at)
			 VALUES('s1',?,?,?,?)`, r[0], r[1], r[2], r[3]); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return path
}

func TestModelDetailWithBreakdown(t *testing.T) {
	// Synthetic cost figures chosen only to exercise the tokenCount *
	// costPerBatch / batchSize formula; batchSize is 1000 throughout.
	path := makeDBWithBreakdown(t, [][4]any{
		{"test-model", int64(700_000),
			`[{"batchSize":1000,"costPerBatch":500000,"tokenCount":400,"tokenType":"input"},` +
				`{"batchSize":1000,"costPerBatch":50000,"tokenCount":0,"tokenType":"cache_read"},` +
				`{"batchSize":1000,"costPerBatch":5000000,"tokenCount":100,"tokenType":"output"}]`,
			"2026-07-25T01:00:00.000Z"},
		{"test-model", int64(940_000),
			`[{"batchSize":1000,"costPerBatch":500000,"tokenCount":600,"tokenType":"input"},` +
				`{"batchSize":1000,"costPerBatch":50000,"tokenCount":800,"tokenType":"cache_read"},` +
				`{"batchSize":1000,"costPerBatch":5000000,"tokenCount":120,"tokenType":"output"}]`,
			"2026-07-25T01:02:00.000Z"},
		// A row predating per-category cost breakdown falls back to "other".
		{"test-model", int64(100_000), nil, "2026-07-25T01:03:00.000Z"},
	})
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	d, err := st.ModelDetail(context.Background(), "test-model")
	if err != nil {
		t.Fatalf("model detail: %v", err)
	}
	wantAIU := (700_000.0 + 940_000.0 + 100_000.0) / nanoPerAIU
	if d.AIU != wantAIU || d.Rows != 3 {
		t.Fatalf("aiu/rows = %v/%d, want %v/3", d.AIU, d.Rows, wantAIU)
	}

	byCat := map[string]float64{}
	for _, c := range d.ByCategory {
		byCat[c.Category] = c.AIU
	}
	wantInput := (200_000.0 + 300_000.0) / nanoPerAIU  // 400*500 + 600*500
	wantCacheRead := (0.0 + 40_000.0) / nanoPerAIU     // 0 + 800*50
	wantOutput := (500_000.0 + 600_000.0) / nanoPerAIU // 100*5000 + 120*5000
	wantOther := 100_000.0 / nanoPerAIU
	if got := byCat["input"]; !floatsClose(got, wantInput) {
		t.Fatalf("input = %v, want %v", got, wantInput)
	}
	if got := byCat["cache_read"]; !floatsClose(got, wantCacheRead) {
		t.Fatalf("cache_read = %v, want %v", got, wantCacheRead)
	}
	if got := byCat["output"]; !floatsClose(got, wantOutput) {
		t.Fatalf("output = %v, want %v", got, wantOutput)
	}
	if got := byCat["other"]; !floatsClose(got, wantOther) {
		t.Fatalf("other = %v, want %v", got, wantOther)
	}

	// Categories reconcile back to the total.
	var sum float64
	for _, v := range byCat {
		sum += v
	}
	if !floatsClose(sum, wantAIU) {
		t.Fatalf("category sum = %v, want %v (total AIU)", sum, wantAIU)
	}

	// Fixed display order: input, cache_read, cache_write, output, then
	// unknown categories ("other") last.
	order := make([]string, len(d.ByCategory))
	for i, c := range d.ByCategory {
		order[i] = c.Category
	}
	if len(order) != 4 || order[0] != "input" || order[1] != "cache_read" || order[2] != "output" || order[3] != "other" {
		t.Fatalf("category order = %v, want [input cache_read output other]", order)
	}
}

func floatsClose(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
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
