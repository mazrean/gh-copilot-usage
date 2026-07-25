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
	if _, err := db.Exec(`CREATE TABLE turns (
		id INTEGER PRIMARY KEY, session_id TEXT, turn_index INTEGER, user_message TEXT)`); err != nil {
		t.Fatalf("create turns: %v", err)
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

// insertTurn adds a turns row for a session.
func insertTurn(t *testing.T, path, sessionID string, turnIndex int, userMessage string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(
		`INSERT INTO turns(session_id, turn_index, user_message) VALUES(?,?,?)`,
		sessionID, turnIndex, userMessage); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
}

// insertUsageEvent adds an assistant_usage_events row with an explicit
// turn_index (nil turnIndex stores SQL NULL).
func insertUsageEvent(t *testing.T, path, sessionID string, turnIndex *int, model string, nanoAIU int64, createdAt string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	var ti sql.NullInt64
	if turnIndex != nil {
		ti = sql.NullInt64{Int64: int64(*turnIndex), Valid: true}
	}
	if _, err := db.Exec(
		`INSERT INTO assistant_usage_events(session_id, turn_index, model, total_nano_aiu, created_at)
		 VALUES(?,?,?,?,?)`, sessionID, ti, model, nanoAIU, createdAt); err != nil {
		t.Fatalf("insert usage event: %v", err)
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
	path := makeDB(t, nil)
	insertSession(t, path, "s1", "Fix login bug", "example-org/example-repo", "main")
	turn0, turn1 := 0, 1
	insertUsageEvent(t, path, "s1", &turn0, "gpt-5-mini", 500_000_000, "2026-07-25T01:00:00.000Z")
	insertUsageEvent(t, path, "s1", &turn1, "claude", 1_000_000_000, "2026-07-25T02:00:00.000Z")
	insertTurn(t, path, "s1", 0, "fix the login bug please")
	insertTurn(t, path, "s1", 1, "now add tests")

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
	if len(d.Turns) != 2 {
		t.Fatalf("turns = %+v, want 2", d.Turns)
	}
	if d.Turns[0].TurnIndex != 0 || d.Turns[0].AIU != 0.5 || d.Turns[0].UserMessage != "fix the login bug please" {
		t.Fatalf("turn 0 = %+v", d.Turns[0])
	}
	if len(d.Turns[0].ByModel) != 1 || d.Turns[0].ByModel[0].Model != "gpt-5-mini" {
		t.Fatalf("turn 0 byModel = %+v", d.Turns[0].ByModel)
	}
	if d.Turns[1].TurnIndex != 1 || d.Turns[1].AIU != 1.0 || d.Turns[1].UserMessage != "now add tests" {
		t.Fatalf("turn 1 = %+v", d.Turns[1])
	}
}

func TestSessionDetailUnassignedTurn(t *testing.T) {
	path := makeDB(t, nil)
	insertSession(t, path, "s1", "", "", "")
	turn0 := 0
	insertUsageEvent(t, path, "s1", &turn0, "claude", 1_000_000_000, "2026-07-25T02:00:00.000Z")
	insertUsageEvent(t, path, "s1", nil, "gpt-5-mini", 500_000_000, "2026-07-25T01:00:00.000Z")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	d, err := st.SessionDetail(context.Background(), "s1")
	if err != nil {
		t.Fatalf("session detail: %v", err)
	}
	if len(d.Turns) != 2 {
		t.Fatalf("turns = %+v, want 2", d.Turns)
	}
	if d.Turns[0].TurnIndex != 0 || d.Turns[0].AIU != 1.0 {
		t.Fatalf("turn 0 = %+v", d.Turns[0])
	}
	if d.Turns[1].TurnIndex != -1 || d.Turns[1].AIU != 0.5 || d.Turns[1].UserMessage != "" {
		t.Fatalf("unassigned turn = %+v", d.Turns[1])
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

// makeDBWithTurnDetail creates a session-store-like DB whose
// assistant_usage_events and turns tables include the per-event detail
// columns (token counts, latency, finish_reason) and turn content columns
// (assistant_response, timestamp) added together in a later session-store.db
// schema version.
func makeDBWithTurnDetail(t *testing.T) string {
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
		session_id TEXT, turn_index INTEGER, agent_id TEXT, parent_tool_call_id TEXT,
		initiator TEXT, model TEXT,
		input_tokens INTEGER, output_tokens INTEGER, cache_read_tokens INTEGER,
		cache_write_tokens INTEGER, reasoning_tokens INTEGER,
		total_nano_aiu INTEGER, duration_ms INTEGER, time_to_first_token_ms REAL,
		finish_reason TEXT, created_at TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions (
		id TEXT PRIMARY KEY, cwd TEXT, repository TEXT, branch TEXT,
		summary TEXT, created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE turns (
		id INTEGER PRIMARY KEY, session_id TEXT, turn_index INTEGER,
		user_message TEXT, assistant_response TEXT, timestamp TEXT)`); err != nil {
		t.Fatalf("create turns: %v", err)
	}
	return path
}

// insertUsageEventDetail adds an assistant_usage_events row with per-event
// detail columns populated. agentID, parentToolCallID and initiator are
// empty/"" for a call made directly by the main agent.
func insertUsageEventDetail(t *testing.T, path, sessionID string, turnIndex int, agentID, parentToolCallID, initiator, model string,
	input, output, cacheRead, cacheWrite, reasoning, nanoAIU, durationMs int64, ttft float64, finishReason, createdAt string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(
		`INSERT INTO assistant_usage_events(
			session_id, turn_index, agent_id, parent_tool_call_id, initiator, model,
			input_tokens, output_tokens,
			cache_read_tokens, cache_write_tokens, reasoning_tokens,
			total_nano_aiu, duration_ms, time_to_first_token_ms, finish_reason, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sessionID, turnIndex, nullIfEmpty(agentID), nullIfEmpty(parentToolCallID), nullIfEmpty(initiator), model,
		input, output, cacheRead, cacheWrite, reasoning,
		nanoAIU, durationMs, ttft, finishReason, createdAt); err != nil {
		t.Fatalf("insert usage event detail: %v", err)
	}
}

// nullIfEmpty maps "" to a SQL NULL so tests can distinguish "column absent"
// from "column set to empty string" the same way real session-store.db rows do.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// insertTurnDetail adds a turns row with assistant_response/timestamp populated.
func insertTurnDetail(t *testing.T, path, sessionID string, turnIndex int, userMessage, assistantResponse, timestamp string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(
		`INSERT INTO turns(session_id, turn_index, user_message, assistant_response, timestamp) VALUES(?,?,?,?,?)`,
		sessionID, turnIndex, userMessage, assistantResponse, timestamp); err != nil {
		t.Fatalf("insert turn detail: %v", err)
	}
}

func TestSessionDetailWithTurnDetailColumns(t *testing.T) {
	path := makeDBWithTurnDetail(t)
	insertSession(t, path, "s1", "Fix login bug", "", "")
	insertUsageEventDetail(t, path, "s1", 0, "", "", "user", "gpt-5-mini",
		100, 50, 20, 0, 10, 500_000_000, 1200, 300.5, "stop", "2026-07-25T01:00:00.000Z")
	insertUsageEventDetail(t, path, "s1", 0, "", "", "agent", "gpt-5-mini",
		200, 60, 0, 5, 0, 300_000_000, 800, 150.0, "tool_calls", "2026-07-25T01:00:05.000Z")
	insertTurnDetail(t, path, "s1", 0, "fix the login bug please", "Done, fixed it.", "2026-07-25T01:00:10.000Z")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	d, err := st.SessionDetail(context.Background(), "s1")
	if err != nil {
		t.Fatalf("session detail: %v", err)
	}
	if len(d.Turns) != 1 {
		t.Fatalf("turns = %+v, want 1", d.Turns)
	}
	turn := d.Turns[0]
	if turn.AssistantResponse != "Done, fixed it." {
		t.Fatalf("assistantResponse = %q", turn.AssistantResponse)
	}
	if turn.Timestamp != "2026-07-25T01:00:10.000Z" {
		t.Fatalf("timestamp = %q", turn.Timestamp)
	}
	if turn.DurationMs != 2000 {
		t.Fatalf("durationMs = %d, want 2000", turn.DurationMs)
	}
	wantTTFT := (300.5 + 150.0) / 2
	if !floatsClose(turn.TimeToFirstTokenMs, wantTTFT) {
		t.Fatalf("ttft = %v, want %v", turn.TimeToFirstTokenMs, wantTTFT)
	}
	if turn.FinishReason != "tool_calls" {
		t.Fatalf("finishReason = %q, want last event's tool_calls", turn.FinishReason)
	}
	if len(turn.ByModel) != 1 {
		t.Fatalf("byModel = %+v, want 1 entry", turn.ByModel)
	}
	tok := turn.ByModel[0].Tokens
	if tok == nil {
		t.Fatalf("tokens = nil, want populated")
	}
	if tok.InputTokens != 300 || tok.OutputTokens != 110 || tok.CacheReadTokens != 20 ||
		tok.CacheWriteTokens != 5 || tok.ReasoningTokens != 10 {
		t.Fatalf("tokens = %+v", tok)
	}

	if len(turn.Spans) != 2 {
		t.Fatalf("spans = %+v, want 2", turn.Spans)
	}
	if turn.Spans[0].Initiator != "user" || turn.Spans[0].AgentID != "" || turn.Spans[0].DurationMs != 1200 {
		t.Fatalf("spans[0] = %+v", turn.Spans[0])
	}
	if turn.Spans[1].Initiator != "agent" || turn.Spans[1].AgentID != "" || turn.Spans[1].DurationMs != 800 {
		t.Fatalf("spans[1] = %+v, want event-order (id ASC), not aggregated", turn.Spans[1])
	}
	if turn.Spans[1].FinishReason != "tool_calls" {
		t.Fatalf("spans[1].FinishReason = %q", turn.Spans[1].FinishReason)
	}
}

// TestSessionDetailSpansWithSubAgent verifies that a sub-agent delegation
// event (agent_id/parent_tool_call_id populated, per the Copilot CLI
// custom-agent delegation feature) is passed through on its span rather than
// being dropped or merged into the main agent's calls.
func TestSessionDetailSpansWithSubAgent(t *testing.T) {
	path := makeDBWithTurnDetail(t)
	insertSession(t, path, "s1", "Review the PR", "", "")
	insertUsageEventDetail(t, path, "s1", 0, "", "", "user", "gpt-5-mini",
		50, 20, 0, 0, 0, 200_000_000, 900, 200, "tool_calls", "2026-07-25T01:00:00.000Z")
	insertUsageEventDetail(t, path, "s1", 0, "react-reviewer", "tc_delegate_1", "agent", "gpt-5-mini",
		400, 150, 0, 0, 5, 900_000_000, 3000, 250, "stop", "2026-07-25T01:00:02.000Z")
	insertUsageEventDetail(t, path, "s1", 0, "", "", "agent", "gpt-5-mini",
		30, 10, 0, 0, 0, 100_000_000, 400, 100, "stop", "2026-07-25T01:00:06.000Z")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	d, err := st.SessionDetail(context.Background(), "s1")
	if err != nil {
		t.Fatalf("session detail: %v", err)
	}
	if len(d.Turns) != 1 {
		t.Fatalf("turns = %+v, want 1", d.Turns)
	}
	spans := d.Turns[0].Spans
	if len(spans) != 3 {
		t.Fatalf("spans = %+v, want 3", spans)
	}
	if spans[0].AgentID != "" {
		t.Fatalf("spans[0].AgentID = %q, want main agent (empty)", spans[0].AgentID)
	}
	if spans[1].AgentID != "react-reviewer" || spans[1].ParentToolCallID != "tc_delegate_1" {
		t.Fatalf("spans[1] = %+v, want sub-agent delegation passed through", spans[1])
	}
	if spans[2].AgentID != "" {
		t.Fatalf("spans[2].AgentID = %q, want main agent resumed (empty)", spans[2].AgentID)
	}
}

func TestSessionDetailWithoutTurnDetailColumns(t *testing.T) {
	// makeDB's fixture schema has neither the per-event detail columns nor
	// the turns content columns, matching an older Copilot CLI session-store.db.
	path := makeDB(t, nil)
	insertSession(t, path, "s1", "", "", "")
	turn0 := 0
	insertUsageEvent(t, path, "s1", &turn0, "gpt-5-mini", 500_000_000, "2026-07-25T01:00:00.000Z")
	insertTurn(t, path, "s1", 0, "fix the login bug please")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	d, err := st.SessionDetail(context.Background(), "s1")
	if err != nil {
		t.Fatalf("session detail: %v", err)
	}
	if len(d.Turns) != 1 {
		t.Fatalf("turns = %+v, want 1", d.Turns)
	}
	turn := d.Turns[0]
	if turn.AssistantResponse != "" || turn.Timestamp != "" || turn.DurationMs != 0 ||
		turn.TimeToFirstTokenMs != 0 || turn.FinishReason != "" {
		t.Fatalf("turn = %+v, want all new fields zero", turn)
	}
	if len(turn.ByModel) != 1 || turn.ByModel[0].Tokens != nil {
		t.Fatalf("byModel = %+v, want Tokens nil (no detail columns)", turn.ByModel)
	}
	if turn.Spans != nil {
		t.Fatalf("spans = %+v, want nil (no detail columns)", turn.Spans)
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
