package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/schema"
	"github.com/t0mer/tollan/internal/search/query"
)

// Store satisfies the logstore.Store interface.
var _ logstore.Store = (*Store)(nil)

// listDays returns the sorted (ascending) day identifiers that have partitions.
func (s *Store) listDays() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("reading store dir: %w", err)
	}
	var days []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".db") {
			continue
		}
		day := strings.TrimSuffix(name, ".db")
		if _, err := time.Parse(dayLayout, day); err != nil {
			continue
		}
		days = append(days, day)
	}
	sort.Strings(days)
	return days, nil
}

// Days implements logstore.Store.
func (s *Store) Days(_ context.Context) ([]string, error) {
	return s.listDays()
}

// DaySizes implements logstore.Store.
func (s *Store) DaySizes(_ context.Context) (map[string]int64, error) {
	days, err := s.listDays()
	if err != nil {
		return nil, err
	}
	sizes := make(map[string]int64, len(days))
	for _, day := range days {
		var total int64
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if fi, err := os.Stat(s.pathFor(day) + suffix); err == nil {
				total += fi.Size()
			}
		}
		sizes[day] = total
	}
	return sizes, nil
}

// DropBefore deletes whole day partitions older than the cutoff day (UTC).
func (s *Store) DropBefore(_ context.Context, cutoff time.Time) (int, error) {
	cutoffDay := dayOf(cutoff)
	days, err := s.listDays()
	if err != nil {
		return 0, err
	}
	dropped := 0
	for _, day := range days {
		if day >= cutoffDay { // lexical compare works for YYYY-MM-DD
			continue
		}
		s.mu.Lock()
		if db, ok := s.dbs[day]; ok {
			_ = db.Close()
			delete(s.dbs, day)
		}
		s.mu.Unlock()
		for _, suffix := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(s.pathFor(day) + suffix)
		}
		dropped++
	}
	return dropped, nil
}

// DeleteStreamBefore deletes rows of a stream older than cutoff across all
// partitions (per-stream retention). FTS rows are cleaned via the external-content
// delete idiom.
func (s *Store) DeleteStreamBefore(ctx context.Context, streamID string, cutoff time.Time) (int64, error) {
	days, err := s.listDays()
	if err != nil {
		return 0, err
	}
	cutMs := cutoff.UTC().UnixMilli()
	var total int64
	for _, day := range days {
		db, err := s.db(day)
		if err != nil {
			return total, err
		}
		// Keep the FTS index consistent: delete matching FTS rows first, then the
		// base rows.
		if _, err := db.ExecContext(ctx,
			`INSERT INTO messages_fts(messages_fts, rowid, body)
			 SELECT 'delete', rowid, body FROM messages WHERE stream = ? AND ts < ?`,
			streamID, cutMs); err != nil {
			return total, fmt.Errorf("pruning fts: %w", err)
		}
		res, err := db.ExecContext(ctx,
			`DELETE FROM messages WHERE stream = ? AND ts < ?`, streamID, cutMs)
		if err != nil {
			return total, fmt.Errorf("pruning stream: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// Search implements logstore.Store by fanning out over the partitions in range.
func (s *Store) Search(ctx context.Context, q logstore.Query) (logstore.Result, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	order := q.Order
	if order != logstore.Ascending {
		order = logstore.Descending
	}

	days, err := s.daysInRange(q.From, q.To)
	if err != nil {
		return logstore.Result{}, err
	}

	fromMs, toMs := boundsMillis(q.From, q.To)
	var all []*schema.Message
	total := 0
	// Fetch up to offset+limit from each partition, then merge.
	perDay := q.Offset + limit
	for _, day := range days {
		db, err := s.db(day)
		if err != nil {
			return logstore.Result{}, err
		}
		msgs, err := searchDay(ctx, db, q, fromMs, toMs, order, perDay)
		if err != nil {
			return logstore.Result{}, err
		}
		all = append(all, msgs...)
		c, err := countDay(ctx, db, q, fromMs, toMs)
		if err != nil {
			return logstore.Result{}, err
		}
		total += c
	}

	sortMessages(all, order)
	// Apply global offset/limit after the merge.
	if q.Offset < len(all) {
		all = all[q.Offset:]
	} else {
		all = nil
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return logstore.Result{Messages: all, Total: total}, nil
}

// daysInRange returns the existing partition days overlapping [from,to]. Zero
// bounds mean open-ended.
func (s *Store) daysInRange(from, to time.Time) ([]string, error) {
	days, err := s.listDays()
	if err != nil {
		return nil, err
	}
	var fromDay, toDay string
	if !from.IsZero() {
		fromDay = dayOf(from)
	}
	if !to.IsZero() {
		toDay = dayOf(to)
	}
	var out []string
	for _, d := range days {
		if fromDay != "" && d < fromDay {
			continue
		}
		if toDay != "" && d > toDay {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func boundsMillis(from, to time.Time) (int64, int64) {
	var lo int64 = -1 << 62
	var hi int64 = 1 << 62
	if !from.IsZero() {
		lo = from.UTC().UnixMilli()
	}
	if !to.IsZero() {
		hi = to.UTC().UnixMilli()
	}
	return lo, hi
}

func orderSQL(order logstore.Direction) string {
	if order == logstore.Ascending {
		return "ASC"
	}
	return "DESC"
}

// whereClause builds the shared WHERE fragment (compiled expression + time range
// + stream) and its args.
func whereClause(q logstore.Query, fromMs, toMs int64) (string, []any, error) {
	expr, exprArgs, err := compileQuery(q)
	if err != nil {
		return "", nil, err
	}
	sb := strings.Builder{}
	args := make([]any, 0, len(exprArgs)+3)

	sb.WriteString("(")
	sb.WriteString(expr)
	sb.WriteString(") AND m.ts BETWEEN ? AND ?")
	args = append(args, exprArgs...)
	args = append(args, fromMs, toMs)
	if q.Stream != "" {
		sb.WriteString(" AND m.stream = ?")
		args = append(args, q.Stream)
	}
	return sb.String(), args, nil
}

// compileQuery resolves the effective query expression: the AST if present,
// otherwise a plain full-text term from Text, otherwise match-all.
func compileQuery(q logstore.Query) (string, []any, error) {
	node := q.Expr
	if node == nil {
		if q.Text != "" {
			node = &query.Term{Value: q.Text}
		} else {
			node = query.MatchAll{}
		}
	}
	return compileExpr(node)
}

func searchDay(ctx context.Context, db *sql.DB, q logstore.Query, fromMs, toMs int64, order logstore.Direction, limit int) ([]*schema.Message, error) {
	where, args, err := whereClause(q, fromMs, toMs)
	if err != nil {
		return nil, err
	}
	sql := "SELECT m.id, m.ts, m.received, m.source, m.stream, m.input_id, m.body, m.fields " +
		"FROM messages m WHERE " + where +
		fmt.Sprintf(" ORDER BY m.ts %s LIMIT ?", orderSQL(order))
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// Histogram implements logstore.Store by bucketing match counts per interval and
// merging across day partitions.
func (s *Store) Histogram(ctx context.Context, q logstore.Query, intervalMillis int64) ([]logstore.Bucket, error) {
	if intervalMillis <= 0 {
		return nil, fmt.Errorf("histogram interval must be positive")
	}
	days, err := s.daysInRange(q.From, q.To)
	if err != nil {
		return nil, err
	}
	fromMs, toMs := boundsMillis(q.From, q.To)
	counts := make(map[int64]int)
	for _, day := range days {
		db, err := s.db(day)
		if err != nil {
			return nil, err
		}
		if err := histogramDay(ctx, db, q, fromMs, toMs, intervalMillis, counts); err != nil {
			return nil, err
		}
	}
	buckets := make([]logstore.Bucket, 0, len(counts))
	for start, c := range counts {
		buckets = append(buckets, logstore.Bucket{StartMillis: start, Count: c})
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].StartMillis < buckets[j].StartMillis })
	return buckets, nil
}

func histogramDay(ctx context.Context, db *sql.DB, q logstore.Query, fromMs, toMs, interval int64, counts map[int64]int) error {
	where, args, err := whereClause(q, fromMs, toMs)
	if err != nil {
		return err
	}
	// Floor each timestamp to its bucket start.
	sql := fmt.Sprintf(
		"SELECT (m.ts/%d)*%d AS bucket, COUNT(*) FROM messages m WHERE %s GROUP BY bucket",
		interval, interval, where)
	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("histogram query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var start int64
		var c int
		if err := rows.Scan(&start, &c); err != nil {
			return err
		}
		counts[start] += c
	}
	return rows.Err()
}

func countDay(ctx context.Context, db *sql.DB, q logstore.Query, fromMs, toMs int64) (int, error) {
	where, args, err := whereClause(q, fromMs, toMs)
	if err != nil {
		return 0, err
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages m WHERE "+where, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count query: %w", err)
	}
	return n, nil
}

func scanMessages(rows *sql.Rows) ([]*schema.Message, error) {
	var out []*schema.Message
	for rows.Next() {
		var (
			m               schema.Message
			tsMs, recMs     int64
			source, stream  sql.NullString
			inputID, fields sql.NullString
			body            sql.NullString
		)
		if err := rows.Scan(&m.ID, &tsMs, &recMs, &source, &stream, &inputID, &body, &fields); err != nil {
			return nil, err
		}
		m.Timestamp = time.UnixMilli(tsMs).UTC()
		m.ReceivedAt = time.UnixMilli(recMs).UTC()
		m.Source = source.String
		m.Stream = stream.String
		m.InputID = inputID.String
		m.Body = body.String
		if fields.Valid && fields.String != "" && fields.String != "{}" {
			m.Fields = make(map[string]any)
			_ = json.Unmarshal([]byte(fields.String), &m.Fields)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func sortMessages(msgs []*schema.Message, order logstore.Direction) {
	sort.SliceStable(msgs, func(i, j int) bool {
		if order == logstore.Ascending {
			return msgs[i].Timestamp.Before(msgs[j].Timestamp)
		}
		return msgs[i].Timestamp.After(msgs[j].Timestamp)
	})
}
