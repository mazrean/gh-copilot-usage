// Package store reads GitHub Copilot CLI AIC usage from ~/.copilot/session-store.db.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

// ErrSessionNotFound indicates no sessions row exists for a given session id.
var ErrSessionNotFound = errors.New("session not found")

// nanoPerAIU is the scale of the total_nano_aiu column: 1 AIU == 1e9 nano_aiu.
const nanoPerAIU = 1e9

// Dimension is the stacking axis of the chart.
type Dimension string

const (
	DimModel   Dimension = "model"
	DimSession Dimension = "session"
)

// Granularity is the time-bucket size of the x-axis.
type Granularity string

const (
	GranDay   Granularity = "day"
	GranWeek  Granularity = "week"
	GranMonth Granularity = "month"
)

// column returns the SQL column expression used for the given dimension.
func (d Dimension) column() (string, error) {
	switch d {
	case DimModel:
		return "model", nil
	case DimSession:
		return "session_id", nil
	default:
		return "", fmt.Errorf("unknown dimension %q", d)
	}
}

// bucketExpr returns a strftime expression that buckets created_at.
func (g Granularity) bucketExpr() (string, error) {
	switch g {
	case GranDay:
		return "strftime('%Y-%m-%d', created_at)", nil
	case GranWeek:
		// Bucket by the Monday (start of week) date, so the frontend can
		// render an actual date range instead of an opaque week number.
		return "date(created_at, '-6 days', 'weekday 1')", nil
	case GranMonth:
		return "strftime('%Y-%m', created_at)", nil
	default:
		return "", fmt.Errorf("unknown granularity %q", g)
	}
}

// Series is one stacked series (one model or one session) across all buckets.
type Series struct {
	Key    string    `json:"key"`
	Label  string    `json:"label"`
	Values []float64 `json:"values"`
}

// Usage is the aggregated result served to the frontend.
type Usage struct {
	Unit        string      `json:"unit"`
	Dimension   Dimension   `json:"dimension"`
	Granularity Granularity `json:"granularity"`
	Buckets     []string    `json:"buckets"`
	Series      []Series    `json:"series"`
	TotalAIU    float64     `json:"totalAIU"`
	Rows        int         `json:"rows"`
	FirstAt     string      `json:"firstAt"`
	LastAt      string      `json:"lastAt"`
}

// Store reads AIC usage from a Copilot session-store SQLite DB.
type Store struct {
	db                    *sql.DB
	cleanup               func()
	hasTokenBreakdown     bool
	hasEventDetailColumns bool
	hasTurnContentColumns bool
}

// DefaultDBPath returns ~/.copilot/session-store.db.
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".copilot", "session-store.db"), nil
}

// Open opens the session-store DB read-only. The Copilot CLI keeps the DB open
// in WAL mode while running; a read-only connection coexists with that. If the
// read-only open fails (e.g. locked WAL), it falls back to reading a temporary
// copy of the DB and its -wal/-shm sidecars.
func Open(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("session-store.db not found: %w", err)
	}

	dsn := "file:" + abs + "?mode=ro&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err == nil {
		if err = probe(db); err == nil {
			st, serr := newStore(db, nil)
			if serr != nil {
				_ = db.Close()
				return nil, serr
			}
			return st, nil
		}
		_ = db.Close()
	}

	// Fallback: work on a temporary snapshot copy.
	tmpDir, err := os.MkdirTemp("", "gh-copilot-usage-*")
	if err != nil {
		return nil, err
	}
	tmpDB := filepath.Join(tmpDir, "session-store.db")
	if cerr := copySnapshot(abs, tmpDB); cerr != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("snapshot copy failed: %w", cerr)
	}
	db2, err := sql.Open("sqlite", "file:"+tmpDB+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	if err := probe(db2); err != nil {
		_ = db2.Close()
		os.RemoveAll(tmpDir)
		return nil, err
	}
	st, serr := newStore(db2, func() { os.RemoveAll(tmpDir) })
	if serr != nil {
		_ = db2.Close()
		os.RemoveAll(tmpDir)
		return nil, serr
	}
	return st, nil
}

// newStore builds a Store from an already-opened, already-probed db,
// detecting which optional schema columns are present.
func newStore(db *sql.DB, cleanup func()) (*Store, error) {
	hasTokenBreakdown, err := hasColumn(db, "assistant_usage_events", "token_details_json")
	if err != nil {
		return nil, err
	}
	// duration_ms is the representative column for the batch of per-event
	// detail columns (token counts, latency, finish_reason, initiator,
	// reasoning_effort) added together in the same session-store.db migration.
	hasEventDetailColumns, err := hasColumn(db, "assistant_usage_events", "duration_ms")
	if err != nil {
		return nil, err
	}
	// assistant_response is the representative column for the turns-table
	// additions (assistant_response, timestamp).
	hasTurnContentColumns, err := hasColumn(db, "turns", "assistant_response")
	if err != nil {
		return nil, err
	}
	return &Store{
		db:                    db,
		cleanup:               cleanup,
		hasTokenBreakdown:     hasTokenBreakdown,
		hasEventDetailColumns: hasEventDetailColumns,
		hasTurnContentColumns: hasTurnContentColumns,
	}, nil
}

// probe verifies the expected table exists.
func probe(db *sql.DB) error {
	var name string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='assistant_usage_events'",
	).Scan(&name)
	if err != nil {
		return fmt.Errorf("assistant_usage_events table not readable: %w", err)
	}
	return nil
}

// hasColumn reports whether table has the given column, so callers can
// degrade gracefully against session-store.db schemas from older Copilot CLI
// versions that predate a column. table is always an internal literal, never
// user input.
func hasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// copySnapshot copies the DB and its WAL sidecars to dst.
func copySnapshot(src, dst string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		s := src + suffix
		if _, err := os.Stat(s); err != nil {
			continue // sidecar may be absent
		}
		if err := copyFile(s, dst+suffix); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// Close releases the DB and any temporary snapshot.
func (s *Store) Close() error {
	err := s.db.Close()
	if s.cleanup != nil {
		s.cleanup()
	}
	return err
}

// Aggregate returns AIC usage bucketed by time and stacked by dimension.
func (s *Store) Aggregate(ctx context.Context, dim Dimension, gran Granularity) (*Usage, error) {
	col, err := dim.column()
	if err != nil {
		return nil, err
	}
	bucket, err := gran.bucketExpr()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT %s AS bucket,
		       COALESCE(NULLIF(%s, ''), 'unknown') AS series,
		       SUM(total_nano_aiu) AS nano
		FROM assistant_usage_events
		WHERE created_at IS NOT NULL
		GROUP BY bucket, series
		ORDER BY bucket ASC`, bucket, col)

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// cell[bucket][series] = AIU
	type cellKey struct{ bucket, series string }
	cells := map[cellKey]float64{}
	bucketOrder := []string{}
	bucketSeen := map[string]bool{}
	seriesTotal := map[string]float64{}

	for rows.Next() {
		var bkt, ser string
		var nano sql.NullInt64
		if err := rows.Scan(&bkt, &ser, &nano); err != nil {
			return nil, err
		}
		aiu := float64(nano.Int64) / nanoPerAIU
		cells[cellKey{bkt, ser}] += aiu
		seriesTotal[ser] += aiu
		if !bucketSeen[bkt] {
			bucketSeen[bkt] = true
			bucketOrder = append(bucketOrder, bkt)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Series ordered by total AIU descending (stable by key for ties).
	seriesKeys := make([]string, 0, len(seriesTotal))
	for k := range seriesTotal {
		seriesKeys = append(seriesKeys, k)
	}
	sort.Slice(seriesKeys, func(i, j int) bool {
		if seriesTotal[seriesKeys[i]] != seriesTotal[seriesKeys[j]] {
			return seriesTotal[seriesKeys[i]] > seriesTotal[seriesKeys[j]]
		}
		return seriesKeys[i] < seriesKeys[j]
	})

	out := &Usage{
		Unit:        "AIU",
		Dimension:   dim,
		Granularity: gran,
		Buckets:     bucketOrder,
		Series:      make([]Series, 0, len(seriesKeys)),
	}
	for _, sk := range seriesKeys {
		vals := make([]float64, len(bucketOrder))
		for i, b := range bucketOrder {
			vals[i] = cells[cellKey{b, sk}]
		}
		out.Series = append(out.Series, Series{Key: sk, Label: sk, Values: vals})
	}

	if dim == DimSession {
		if err := s.fillSessionLabels(ctx, out.Series); err != nil {
			return nil, err
		}
	}

	if err := s.meta(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// fillSessionLabels replaces each session series' Label with a human-readable
// name (the session summary, falling back to "repository (branch)") looked up
// from the sessions table. Series with no matching row, or no usable name,
// keep the raw session_id as their Label.
func (s *Store) fillSessionLabels(ctx context.Context, series []Series) error {
	ids := make([]string, 0, len(series))
	for _, sr := range series {
		if sr.Key != "unknown" {
			ids = append(ids, sr.Key)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT id, summary, repository, branch
		FROM sessions
		WHERE id IN (%s)`, strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	labels := make(map[string]string, len(ids))
	for rows.Next() {
		var id string
		var summary, repository, branch sql.NullString
		if err := rows.Scan(&id, &summary, &repository, &branch); err != nil {
			return err
		}
		if label := sessionLabel(summary, repository, branch); label != "" {
			labels[id] = label
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range series {
		if label, ok := labels[series[i].Key]; ok {
			series[i].Label = label
		}
	}
	return nil
}

// sessionLabel derives a display name from session metadata, preferring the
// session summary and falling back to "repository (branch)". Returns "" when
// no usable name is available.
func sessionLabel(summary, repository, branch sql.NullString) string {
	if summary.Valid && summary.String != "" {
		return summary.String
	}
	if repository.Valid && repository.String != "" {
		if branch.Valid && branch.String != "" {
			return repository.String + " (" + branch.String + ")"
		}
		return repository.String
	}
	return ""
}

// TurnTokenUsage is a raw token-count breakdown (not AIU) for one model
// within one turn, available when the session-store.db exposes the
// assistant_usage_events per-event detail columns.
type TurnTokenUsage struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
	ReasoningTokens  int64 `json:"reasoningTokens"`
}

// SessionModelUsage is one model's AIU contribution within a single session
// (or, within SessionTurnUsage.ByModel, within a single turn). Tokens is nil
// unless the caller populated it from per-event detail columns.
type SessionModelUsage struct {
	Model  string          `json:"model"`
	AIU    float64         `json:"aiu"`
	Rows   int             `json:"rows"`
	Tokens *TurnTokenUsage `json:"tokens,omitempty"`
}

// TurnEventSpan is one raw assistant_usage_events row within a turn,
// preserved at event granularity (rather than summed across the turn like
// SessionTurnUsage's own DurationMs/TimeToFirstTokenMs/FinishReason) so
// callers can reconstruct the sequence of calls that made up the turn.
//
// AgentID and ParentToolCallID are empty for calls made directly by the main
// agent. When Copilot CLI delegates to a custom sub-agent, AgentID identifies
// that sub-agent and ParentToolCallID references the tool call that spawned
// it, letting a caller nest the sub-agent's spans under the main agent's
// timeline. Only populated when the underlying session-store.db exposes the
// per-event detail columns (see hasEventDetailColumns).
type TurnEventSpan struct {
	AgentID          string          `json:"agentId,omitempty"`
	ParentToolCallID string          `json:"parentToolCallId,omitempty"`
	Initiator        string          `json:"initiator,omitempty"`
	Model            string          `json:"model"`
	StartedAt        string          `json:"startedAt,omitempty"`
	DurationMs       int64           `json:"durationMs,omitempty"`
	AIU              float64         `json:"aiu"`
	FinishReason     string          `json:"finishReason,omitempty"`
	Tokens           *TurnTokenUsage `json:"tokens,omitempty"`
}

// SessionTurnUsage is one turn's AIU contribution within a single session,
// broken down by model. TurnIndex is -1 for usage events with no turn_index
// (turn_index IS NULL), grouped together as "unassigned".
//
// AssistantResponse, Timestamp, DurationMs, TimeToFirstTokenMs, FinishReason
// and Spans are only populated when the underlying session-store.db exposes
// the corresponding columns (older schemas leave them zero/empty/nil).
type SessionTurnUsage struct {
	TurnIndex          int                 `json:"turnIndex"`
	AIU                float64             `json:"aiu"`
	UserMessage        string              `json:"userMessage"`
	AssistantResponse  string              `json:"assistantResponse,omitempty"`
	Timestamp          string              `json:"timestamp,omitempty"`
	DurationMs         int64               `json:"durationMs,omitempty"`
	TimeToFirstTokenMs float64             `json:"timeToFirstTokenMs,omitempty"`
	FinishReason       string              `json:"finishReason,omitempty"`
	ByModel            []SessionModelUsage `json:"byModel"`
	Spans              []TurnEventSpan     `json:"spans,omitempty"`
}

// SessionDetail is the drill-down view for a single session: its metadata,
// per-model AIU breakdown, and per-turn AIU breakdown.
type SessionDetail struct {
	ID         string              `json:"id"`
	Summary    string              `json:"summary"`
	Repository string              `json:"repository"`
	Branch     string              `json:"branch"`
	Cwd        string              `json:"cwd"`
	CreatedAt  string              `json:"createdAt"`
	UpdatedAt  string              `json:"updatedAt"`
	ByModel    []SessionModelUsage `json:"byModel"`
	Turns      []SessionTurnUsage  `json:"turns"`
}

// SessionDetail returns metadata, per-model AIU breakdown and per-turn AIU
// breakdown for one session. It returns ErrSessionNotFound if no sessions row
// matches id.
func (s *Store) SessionDetail(ctx context.Context, id string) (*SessionDetail, error) {
	var summary, repository, branch, cwd, createdAt, updatedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT summary, repository, branch, cwd, created_at, updated_at
		FROM sessions
		WHERE id = ?`, id).Scan(&summary, &repository, &branch, &cwd, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}

	detail := &SessionDetail{
		ID:         id,
		Summary:    summary.String,
		Repository: repository.String,
		Branch:     branch.String,
		Cwd:        cwd.String,
		CreatedAt:  createdAt.String,
		UpdatedAt:  updatedAt.String,
	}

	modelRows, err := s.db.QueryContext(ctx, `
		SELECT model, SUM(total_nano_aiu), COUNT(*)
		FROM assistant_usage_events
		WHERE session_id = ?
		GROUP BY model
		ORDER BY SUM(total_nano_aiu) DESC`, id)
	if err != nil {
		return nil, err
	}
	for modelRows.Next() {
		var model string
		var nano sql.NullInt64
		var rows int
		if err := modelRows.Scan(&model, &nano, &rows); err != nil {
			modelRows.Close()
			return nil, err
		}
		detail.ByModel = append(detail.ByModel, SessionModelUsage{
			Model: model,
			AIU:   float64(nano.Int64) / nanoPerAIU,
			Rows:  rows,
		})
	}
	if err := modelRows.Err(); err != nil {
		modelRows.Close()
		return nil, err
	}
	modelRows.Close()

	turnModelQuery := `
		SELECT turn_index, model, SUM(total_nano_aiu), COUNT(*)
		FROM assistant_usage_events
		WHERE session_id = ?
		GROUP BY turn_index, model
		ORDER BY turn_index ASC, SUM(total_nano_aiu) DESC`
	if s.hasEventDetailColumns {
		turnModelQuery = `
			SELECT turn_index, model, SUM(total_nano_aiu), COUNT(*),
			       SUM(input_tokens), SUM(output_tokens), SUM(cache_read_tokens),
			       SUM(cache_write_tokens), SUM(reasoning_tokens)
			FROM assistant_usage_events
			WHERE session_id = ?
			GROUP BY turn_index, model
			ORDER BY turn_index ASC, SUM(total_nano_aiu) DESC`
	}
	turnRows, err := s.db.QueryContext(ctx, turnModelQuery, id)
	if err != nil {
		return nil, err
	}
	var turns []SessionTurnUsage
	var unassigned *SessionTurnUsage
	var current *SessionTurnUsage
	for turnRows.Next() {
		var turnIndex sql.NullInt64
		var model string
		var nano sql.NullInt64
		var rows int
		var modelUsage SessionModelUsage
		if s.hasEventDetailColumns {
			var input, output, cacheRead, cacheWrite, reasoning sql.NullInt64
			if err := turnRows.Scan(&turnIndex, &model, &nano, &rows,
				&input, &output, &cacheRead, &cacheWrite, &reasoning); err != nil {
				turnRows.Close()
				return nil, err
			}
			modelUsage.Tokens = &TurnTokenUsage{
				InputTokens:      input.Int64,
				OutputTokens:     output.Int64,
				CacheReadTokens:  cacheRead.Int64,
				CacheWriteTokens: cacheWrite.Int64,
				ReasoningTokens:  reasoning.Int64,
			}
		} else if err := turnRows.Scan(&turnIndex, &model, &nano, &rows); err != nil {
			turnRows.Close()
			return nil, err
		}
		modelUsage.Model = model
		modelUsage.AIU = float64(nano.Int64) / nanoPerAIU
		modelUsage.Rows = rows
		if !turnIndex.Valid {
			if unassigned == nil {
				unassigned = &SessionTurnUsage{TurnIndex: -1}
			}
			unassigned.AIU += modelUsage.AIU
			unassigned.ByModel = append(unassigned.ByModel, modelUsage)
			continue
		}
		idx := int(turnIndex.Int64)
		if current == nil || current.TurnIndex != idx {
			turns = append(turns, SessionTurnUsage{TurnIndex: idx})
			current = &turns[len(turns)-1]
		}
		current.AIU += modelUsage.AIU
		current.ByModel = append(current.ByModel, modelUsage)
	}
	if err := turnRows.Err(); err != nil {
		turnRows.Close()
		return nil, err
	}
	turnRows.Close()

	if s.hasEventDetailColumns {
		if err := s.fillTurnMeta(ctx, id, turns); err != nil {
			return nil, err
		}
		if err := s.fillTurnSpans(ctx, id, turns); err != nil {
			return nil, err
		}
	}

	msgQuery := `
		SELECT turn_index, COALESCE(user_message, '')
		FROM turns
		WHERE session_id = ?`
	if s.hasTurnContentColumns {
		msgQuery = `
			SELECT turn_index, COALESCE(user_message, ''), COALESCE(assistant_response, ''), COALESCE(timestamp, '')
			FROM turns
			WHERE session_id = ?`
	}
	msgRows, err := s.db.QueryContext(ctx, msgQuery, id)
	if err != nil {
		return nil, err
	}
	type turnContent struct {
		userMessage       string
		assistantResponse string
		timestamp         string
	}
	turnContents := make(map[int]turnContent)
	for msgRows.Next() {
		var turnIndex int
		var c turnContent
		if s.hasTurnContentColumns {
			if err := msgRows.Scan(&turnIndex, &c.userMessage, &c.assistantResponse, &c.timestamp); err != nil {
				msgRows.Close()
				return nil, err
			}
		} else if err := msgRows.Scan(&turnIndex, &c.userMessage); err != nil {
			msgRows.Close()
			return nil, err
		}
		turnContents[turnIndex] = c
	}
	if err := msgRows.Err(); err != nil {
		msgRows.Close()
		return nil, err
	}
	msgRows.Close()

	for i := range turns {
		c := turnContents[turns[i].TurnIndex]
		turns[i].UserMessage = c.userMessage
		turns[i].AssistantResponse = c.assistantResponse
		turns[i].Timestamp = c.timestamp
	}
	if unassigned != nil {
		turns = append(turns, *unassigned)
	}
	detail.Turns = turns

	return detail, nil
}

// fillTurnMeta populates DurationMs, TimeToFirstTokenMs and FinishReason on
// each turn in turns (in place) from assistant_usage_events, aggregating
// across all models/events within each turn. The unassigned bucket
// (TurnIndex -1) is not covered, since it has no single turn_index to
// aggregate by.
func (s *Store) fillTurnMeta(ctx context.Context, sessionID string, turns []SessionTurnUsage) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT turn_index,
		       SUM(duration_ms),
		       AVG(time_to_first_token_ms),
		       (SELECT finish_reason FROM assistant_usage_events e2
		        WHERE e2.session_id = e.session_id AND e2.turn_index = e.turn_index
		        ORDER BY e2.id DESC LIMIT 1)
		FROM assistant_usage_events e
		WHERE session_id = ? AND turn_index IS NOT NULL
		GROUP BY turn_index`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type meta struct {
		durationMs   int64
		ttft         float64
		finishReason string
	}
	byTurn := make(map[int]meta)
	for rows.Next() {
		var turnIndex int
		var duration sql.NullInt64
		var ttft sql.NullFloat64
		var finishReason sql.NullString
		if err := rows.Scan(&turnIndex, &duration, &ttft, &finishReason); err != nil {
			return err
		}
		byTurn[turnIndex] = meta{
			durationMs:   duration.Int64,
			ttft:         ttft.Float64,
			finishReason: finishReason.String,
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range turns {
		m, ok := byTurn[turns[i].TurnIndex]
		if !ok {
			continue
		}
		turns[i].DurationMs = m.durationMs
		turns[i].TimeToFirstTokenMs = m.ttft
		turns[i].FinishReason = m.finishReason
	}
	return nil
}

// fillTurnSpans populates Spans on each turn in turns (in place) with one
// TurnEventSpan per assistant_usage_events row, in the order the events
// occurred (turn_index, id ASC). Unlike fillTurnMeta this does not aggregate
// across events, so callers can see the individual calls (and, once Copilot
// CLI populates agent_id/parent_tool_call_id for sub-agent delegation, their
// hierarchy) that made up a turn. The unassigned bucket (TurnIndex -1) is not
// covered, matching fillTurnMeta.
func (s *Store) fillTurnSpans(ctx context.Context, sessionID string, turns []SessionTurnUsage) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT turn_index, model, COALESCE(created_at, ''), duration_ms, total_nano_aiu,
		       COALESCE(agent_id, ''), COALESCE(parent_tool_call_id, ''),
		       COALESCE(initiator, ''), COALESCE(finish_reason, ''),
		       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens
		FROM assistant_usage_events
		WHERE session_id = ? AND turn_index IS NOT NULL
		ORDER BY turn_index ASC, id ASC`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()

	byTurn := make(map[int][]TurnEventSpan)
	for rows.Next() {
		var turnIndex int
		var span TurnEventSpan
		var duration sql.NullInt64
		var nano sql.NullInt64
		var input, output, cacheRead, cacheWrite, reasoning sql.NullInt64
		if err := rows.Scan(&turnIndex, &span.Model, &span.StartedAt, &duration, &nano,
			&span.AgentID, &span.ParentToolCallID, &span.Initiator, &span.FinishReason,
			&input, &output, &cacheRead, &cacheWrite, &reasoning); err != nil {
			return err
		}
		span.DurationMs = duration.Int64
		span.AIU = float64(nano.Int64) / nanoPerAIU
		span.Tokens = &TurnTokenUsage{
			InputTokens:      input.Int64,
			OutputTokens:     output.Int64,
			CacheReadTokens:  cacheRead.Int64,
			CacheWriteTokens: cacheWrite.Int64,
			ReasoningTokens:  reasoning.Int64,
		}
		byTurn[turnIndex] = append(byTurn[turnIndex], span)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range turns {
		turns[i].Spans = byTurn[turns[i].TurnIndex]
	}
	return nil
}

// categoryOrder fixes the display order of known token-cost categories so the
// frontend can assign stable colors regardless of which categories a given
// model actually used.
var categoryOrder = []string{"input", "cache_read", "cache_write", "output"}

func categoryRank(category string) int {
	for i, c := range categoryOrder {
		if c == category {
			return i
		}
	}
	return len(categoryOrder) // unknown/"other" categories sort last
}

// ModelCategoryUsage is one token-cost category's (input, cached input,
// cache write, output, ...) AIU contribution for a single model.
type ModelCategoryUsage struct {
	Category string  `json:"category"`
	AIU      float64 `json:"aiu"`
}

// ModelDetail is the drill-down view for a single model: its total AIU and,
// when the underlying DB exposes per-event cost breakdown, how that AIU
// split across token categories.
type ModelDetail struct {
	Model      string               `json:"model"`
	AIU        float64              `json:"aiu"`
	Rows       int                  `json:"rows"`
	ByCategory []ModelCategoryUsage `json:"byCategory"`
}

// ModelDetail returns total AIU/row count and, when available, a per-token-
// category cost breakdown for one model. Unlike SessionDetail, an unknown
// model is not an error - it simply reports zero usage, since "model" is a
// GROUP BY key rather than an entity with its own existence.
func (s *Store) ModelDetail(ctx context.Context, model string) (*ModelDetail, error) {
	detail := &ModelDetail{Model: model}

	var nano sql.NullInt64
	var rows int
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(total_nano_aiu), 0), COUNT(*)
		FROM assistant_usage_events
		WHERE model = ?`, model).Scan(&nano, &rows)
	if err != nil {
		return nil, err
	}
	detail.AIU = float64(nano.Int64) / nanoPerAIU
	detail.Rows = rows

	if !s.hasTokenBreakdown {
		return detail, nil
	}

	catRows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(je.value ->> 'tokenType', 'other') AS category,
		       SUM(
		         CASE WHEN je.value IS NOT NULL
		              THEN CAST(je.value ->> 'tokenCount' AS REAL) * CAST(je.value ->> 'costPerBatch' AS REAL)
		                   / NULLIF(CAST(je.value ->> 'batchSize' AS REAL), 0)
		              ELSE total_nano_aiu
		         END
		       ) AS nano
		FROM assistant_usage_events
		LEFT JOIN json_each(
		  CASE WHEN json_valid(token_details_json) THEN token_details_json ELSE NULL END
		) je ON 1=1
		WHERE model = ?
		GROUP BY category`, model)
	if err != nil {
		return nil, err
	}
	defer catRows.Close()

	for catRows.Next() {
		var category string
		var catNano sql.NullFloat64
		if err := catRows.Scan(&category, &catNano); err != nil {
			return nil, err
		}
		detail.ByCategory = append(detail.ByCategory, ModelCategoryUsage{
			Category: category,
			AIU:      catNano.Float64 / nanoPerAIU,
		})
	}
	if err := catRows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(detail.ByCategory, func(i, j int) bool {
		ri, rj := categoryRank(detail.ByCategory[i].Category), categoryRank(detail.ByCategory[j].Category)
		if ri != rj {
			return ri < rj
		}
		return detail.ByCategory[i].Category < detail.ByCategory[j].Category
	})

	return detail, nil
}

// meta fills total AIU, row count and the created_at range.
func (s *Store) meta(ctx context.Context, u *Usage) error {
	var nano sql.NullInt64
	var rows int
	var first, last sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(total_nano_aiu),0), COUNT(*), MIN(created_at), MAX(created_at)
		FROM assistant_usage_events`).Scan(&nano, &rows, &first, &last)
	if err != nil {
		return err
	}
	u.TotalAIU = float64(nano.Int64) / nanoPerAIU
	u.Rows = rows
	u.FirstAt = first.String
	u.LastAt = last.String
	return nil
}
