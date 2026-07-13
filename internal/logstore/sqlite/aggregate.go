package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"

	"github.com/t0mer/tollan/internal/logstore"
)

// percentileCap bounds how many values a percentile aggregation scans.
const percentileCap = 200000

// Aggregate groups matching messages and computes a metric per group. count/sum/
// avg/min/max are computed in SQL; percentiles are computed in Go over the
// matching values (bounded).
func (s *Store) Aggregate(ctx context.Context, q logstore.Query, spec logstore.AggSpec) ([]logstore.AggRow, error) {
	limit := spec.Limit
	if limit <= 0 {
		limit = 50
	}
	if spec.GroupBy != "" {
		if err := validateAggField(spec.GroupBy); err != nil {
			return nil, err
		}
	}
	if spec.Metric != logstore.MetricCount {
		if spec.MetricField == "" {
			return nil, fmt.Errorf("metric %q requires a metric field", spec.Metric)
		}
		if err := validateAggField(spec.MetricField); err != nil {
			return nil, err
		}
	}

	if isPercentile(spec.Metric) {
		return s.aggregatePercentile(ctx, q, spec, limit)
	}
	return s.aggregateSQL(ctx, q, spec, limit)
}

func validateAggField(field string) error {
	if _, ok := columnExpr(field); ok {
		return nil
	}
	return validateFieldName(field)
}

func fieldSQL(field string) string {
	if col, ok := columnExpr(field); ok {
		return col
	}
	return jsonExpr(field)
}

func isPercentile(m logstore.AggMetric) bool {
	switch m {
	case logstore.MetricP50, logstore.MetricP90, logstore.MetricP95, logstore.MetricP99:
		return true
	}
	return false
}

func (s *Store) aggregateSQL(ctx context.Context, q logstore.Query, spec logstore.AggSpec, limit int) ([]logstore.AggRow, error) {
	days, err := s.daysInRange(q.From, q.To)
	if err != nil {
		return nil, err
	}
	fromMs, toMs := boundsMillis(q.From, q.To)

	metricExpr := "COUNT(*)"
	switch spec.Metric {
	case logstore.MetricCount:
		metricExpr = "COUNT(*)"
	case logstore.MetricSum:
		metricExpr = "SUM(CAST(" + fieldSQL(spec.MetricField) + " AS REAL))"
	case logstore.MetricAvg:
		metricExpr = "AVG(CAST(" + fieldSQL(spec.MetricField) + " AS REAL))"
	case logstore.MetricMin:
		metricExpr = "MIN(CAST(" + fieldSQL(spec.MetricField) + " AS REAL))"
	case logstore.MetricMax:
		metricExpr = "MAX(CAST(" + fieldSQL(spec.MetricField) + " AS REAL))"
	default:
		return nil, fmt.Errorf("unsupported metric %q", spec.Metric)
	}

	// Accumulate across partitions. For grouped counts/sums we can add; for
	// avg/min/max across partitions we approximate by re-aggregating group keys
	// (avg-of-avg is inexact across days, acceptable for dashboards).
	acc := map[string]*aggAccum{}
	for _, day := range days {
		db, err := s.db(day)
		if err != nil {
			return nil, err
		}
		where, args, err := whereClause(q, fromMs, toMs)
		if err != nil {
			return nil, err
		}
		var query string
		if spec.GroupBy == "" {
			query = "SELECT '' AS k, " + metricExpr + " AS v, COUNT(*) AS c FROM messages m WHERE " + where
		} else {
			gexpr := fieldSQL(spec.GroupBy)
			query = "SELECT " + gexpr + " AS k, " + metricExpr + " AS v, COUNT(*) AS c FROM messages m WHERE " + where + " GROUP BY k"
		}
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("aggregate: %w", err)
		}
		if err := accumulate(rows, spec.Metric, acc); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	out := make([]logstore.AggRow, 0, len(acc))
	for k, a := range acc {
		out = append(out, logstore.AggRow{Key: k, Value: a.value(spec.Metric)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type aggAccum struct {
	sum   float64
	count float64
	min   float64
	max   float64
	hasMM bool
}

func (a *aggAccum) value(m logstore.AggMetric) float64 {
	switch m {
	case logstore.MetricCount, logstore.MetricSum:
		return a.sum
	case logstore.MetricAvg:
		if a.count == 0 {
			return 0
		}
		return a.sum / a.count
	case logstore.MetricMin:
		return a.min
	case logstore.MetricMax:
		return a.max
	default:
		return a.sum
	}
}

func accumulate(rows *sql.Rows, metric logstore.AggMetric, acc map[string]*aggAccum) error {
	for rows.Next() {
		var k sql.NullString
		var v sql.NullFloat64
		var c float64
		if err := rows.Scan(&k, &v, &c); err != nil {
			return err
		}
		key := k.String
		a := acc[key]
		if a == nil {
			a = &aggAccum{}
			acc[key] = a
		}
		switch metric {
		case logstore.MetricCount:
			a.sum += v.Float64 // v is COUNT(*)
		case logstore.MetricSum:
			a.sum += v.Float64
		case logstore.MetricAvg:
			a.sum += v.Float64 * c // weight partial avg by row count
			a.count += c
		case logstore.MetricMin:
			if !a.hasMM || v.Float64 < a.min {
				a.min = v.Float64
			}
			a.hasMM = true
		case logstore.MetricMax:
			if !a.hasMM || v.Float64 > a.max {
				a.max = v.Float64
			}
			a.hasMM = true
		}
	}
	return rows.Err()
}

func (s *Store) aggregatePercentile(ctx context.Context, q logstore.Query, spec logstore.AggSpec, limit int) ([]logstore.AggRow, error) {
	days, err := s.daysInRange(q.From, q.To)
	if err != nil {
		return nil, err
	}
	fromMs, toMs := boundsMillis(q.From, q.To)
	groups := map[string][]float64{}
	total := 0

	for _, day := range days {
		if total >= percentileCap {
			break
		}
		db, err := s.db(day)
		if err != nil {
			return nil, err
		}
		where, args, err := whereClause(q, fromMs, toMs)
		if err != nil {
			return nil, err
		}
		gexpr := "''"
		if spec.GroupBy != "" {
			gexpr = fieldSQL(spec.GroupBy)
		}
		query := "SELECT " + gexpr + " AS k, CAST(" + fieldSQL(spec.MetricField) + " AS REAL) AS v " +
			"FROM messages m WHERE " + where + " LIMIT ?"
		rows, err := db.QueryContext(ctx, query, append(args, percentileCap)...)
		if err != nil {
			return nil, fmt.Errorf("percentile: %w", err)
		}
		for rows.Next() {
			var k sql.NullString
			var v sql.NullFloat64
			if err := rows.Scan(&k, &v); err != nil {
				rows.Close()
				return nil, err
			}
			if v.Valid {
				groups[k.String] = append(groups[k.String], v.Float64)
				total++
			}
		}
		rows.Close()
	}

	p := percentileFraction(spec.Metric)
	out := make([]logstore.AggRow, 0, len(groups))
	for k, vals := range groups {
		out = append(out, logstore.AggRow{Key: k, Value: percentile(vals, p)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func percentileFraction(m logstore.AggMetric) float64 {
	switch m {
	case logstore.MetricP50:
		return 0.50
	case logstore.MetricP90:
		return 0.90
	case logstore.MetricP95:
		return 0.95
	case logstore.MetricP99:
		return 0.99
	default:
		return 0.50
	}
}

func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sort.Float64s(vals)
	idx := int(math.Ceil(p*float64(len(vals)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(vals) {
		idx = len(vals) - 1
	}
	return vals[idx]
}
