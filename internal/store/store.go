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
		// ISO-ish week label. %W treats Monday as the first day of the week.
		return "strftime('%Y-W%W', created_at)", nil
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
	db                *sql.DB
	cleanup           func()
	hasTokenBreakdown bool
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
			hasBreakdown, herr := hasColumn(db, "assistant_usage_events", "token_details_json")
			if herr != nil {
				_ = db.Close()
				return nil, herr
			}
			return &Store{db: db, hasTokenBreakdown: hasBreakdown}, nil
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
	hasBreakdown, herr := hasColumn(db2, "assistant_usage_events", "token_details_json")
	if herr != nil {
		_ = db2.Close()
		os.RemoveAll(tmpDir)
		return nil, herr
	}
	return &Store{db: db2, cleanup: func() { os.RemoveAll(tmpDir) }, hasTokenBreakdown: hasBreakdown}, nil
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

// SessionModelUsage is one model's AIU contribution within a single session.
type SessionModelUsage struct {
	Model string  `json:"model"`
	AIU   float64 `json:"aiu"`
	Rows  int     `json:"rows"`
}

// SessionCheckpoint is one recorded checkpoint summary within a session.
type SessionCheckpoint struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Overview  string `json:"overview"`
	WorkDone  string `json:"workDone"`
	NextSteps string `json:"nextSteps"`
	CreatedAt string `json:"createdAt"`
}

// SessionDetail is the drill-down view for a single session: its metadata,
// per-model AIU breakdown, and checkpoint summaries.
type SessionDetail struct {
	ID          string              `json:"id"`
	Summary     string              `json:"summary"`
	Repository  string              `json:"repository"`
	Branch      string              `json:"branch"`
	Cwd         string              `json:"cwd"`
	CreatedAt   string              `json:"createdAt"`
	UpdatedAt   string              `json:"updatedAt"`
	ByModel     []SessionModelUsage `json:"byModel"`
	Checkpoints []SessionCheckpoint `json:"checkpoints"`
}

// SessionDetail returns metadata, per-model AIU breakdown and checkpoint
// summaries for one session. It returns ErrSessionNotFound if no sessions row
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

	cpRows, err := s.db.QueryContext(ctx, `
		SELECT checkpoint_number, COALESCE(title, ''), COALESCE(overview, ''),
		       COALESCE(work_done, ''), COALESCE(next_steps, ''), created_at
		FROM checkpoints
		WHERE session_id = ?
		ORDER BY checkpoint_number ASC`, id)
	if err != nil {
		return nil, err
	}
	defer cpRows.Close()
	for cpRows.Next() {
		var cp SessionCheckpoint
		if err := cpRows.Scan(&cp.Number, &cp.Title, &cp.Overview, &cp.WorkDone, &cp.NextSteps, &cp.CreatedAt); err != nil {
			return nil, err
		}
		detail.Checkpoints = append(detail.Checkpoints, cp)
	}
	if err := cpRows.Err(); err != nil {
		return nil, err
	}

	return detail, nil
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
