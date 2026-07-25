// Package store reads GitHub Copilot CLI AIC usage from ~/.copilot/session-store.db.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

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
	db      *sql.DB
	cleanup func()
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
			return &Store{db: db}, nil
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
	return &Store{db: db2, cleanup: func() { os.RemoveAll(tmpDir) }}, nil
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
		out.Series = append(out.Series, Series{Key: sk, Values: vals})
	}

	if err := s.meta(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
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
