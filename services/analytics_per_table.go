package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// The monitoring page's figures across every interface's message table,
// read one table per statement. A single statement over all of them (a
// UNION) locks every table and its indexes at once: with many interfaces
// that is thousands of locks, enough to fill PostgreSQL's shared lock table
// and fail other work on the same server. One table per statement holds a
// few locks at a time, released as each statement ends.

// perTableWorkers is how many interface tables are read at once.
const perTableWorkers = 4

// eachMessageTable calls fn for every table, perTableWorkers at a time. A
// table that fails is skipped: the page shows what could be read.
func (s *AnalyticsService) eachMessageTable(ctx context.Context, tables []string, fn func(ctx context.Context, table string)) {
	sem := make(chan struct{}, perTableWorkers)
	var wg sync.WaitGroup
	for _, t := range tables {
		if !safeTableNameRegex.MatchString(t) {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(table string) {
			defer func() { <-sem; wg.Done() }()
			fn(ctx, table)
		}(t)
	}
	wg.Wait()
}

// messageCounts is how many messages every interface received since
// todayStart, and in each of the 7 hours before now (oldest first): one
// statement per table.
func (s *AnalyticsService) messageCounts(ctx context.Context, tables []string, todayStart, now time.Time) (today int64, hourly [7]int64) {
	// Hour k is [bounds[k], bounds[k+1]): from 7 hours ago to now.
	var bounds [8]time.Time
	for k := range bounds {
		bounds[k] = now.Add(-time.Duration(7-k) * time.Hour)
	}
	from := todayStart
	if bounds[0].Before(from) {
		from = bounds[0]
	}
	filters := make([]string, 0, 8)
	filters = append(filters, "COUNT(*) FILTER (WHERE received_at >= $1)")
	for k := 0; k < 7; k++ {
		filters = append(filters, fmt.Sprintf("COUNT(*) FILTER (WHERE received_at >= $%d AND received_at < $%d)", k+2, k+3))
	}
	args := []any{todayStart}
	for _, b := range bounds {
		args = append(args, b)
	}
	args = append(args, from)
	where := fmt.Sprintf("received_at >= $%d", len(args))

	var mu sync.Mutex
	s.eachMessageTable(ctx, tables, func(ctx context.Context, table string) {
		var counts [8]int64
		dest := make([]any, 8)
		for i := range counts {
			dest[i] = &counts[i]
		}
		q := fmt.Sprintf("SELECT %s FROM %s WHERE %s", strings.Join(filters, ", "), table, where)
		if err := s.db.QueryRowContext(ctx, q, args...).Scan(dest...); err != nil {
			return
		}
		mu.Lock()
		today += counts[0]
		for k := 0; k < 7; k++ {
			hourly[k] += counts[k+1]
		}
		mu.Unlock()
	})
	return today, hourly
}

// liveMessages is the most recent messages across every interface, newest
// first: each table's newest few, one statement per table, then the newest
// limit of those, named after their interfaces.
func (s *AnalyticsService) liveMessages(ctx context.Context, tables []string, limit int) []*LiveMessage {
	type row struct {
		m  *LiveMessage
		at time.Time
	}
	perTable := 5
	if limit < perTable {
		perTable = limit
	}
	var mu sync.Mutex
	var rows []row
	s.eachMessageTable(ctx, tables, func(ctx context.Context, table string) {
		ifaceID := rawTableSuffixToUUID(strings.TrimPrefix(table, "messages_intf_"))
		rs, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT message_id, message_type, status, received_at, processing_time_ms
			FROM %s ORDER BY received_at DESC LIMIT %d`, table, perTable))
		if err != nil {
			return
		}
		defer rs.Close()
		var got []row
		for rs.Next() {
			m := &LiveMessage{InterfaceID: ifaceID}
			var at time.Time
			if rs.Scan(&m.MessageID, &m.MessageType, &m.Status, &at, &m.ProcessingTimeMs) == nil {
				m.ReceivedAt = at.Format(time.RFC3339)
				got = append(got, row{m: m, at: at})
			}
		}
		mu.Lock()
		rows = append(rows, got...)
		mu.Unlock()
	})
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].at.After(rows[j].at) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	names := s.interfaceNames(ctx)
	out := make([]*LiveMessage, 0, len(rows))
	for _, r := range rows {
		r.m.InterfaceName = "Unknown"
		if n, ok := names[r.m.InterfaceID]; ok && n != "" {
			r.m.InterfaceName = n
		}
		out = append(out, r.m)
	}
	return out
}

// interfaceNames maps interface IDs to their names.
func (s *AnalyticsService) interfaceNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	rs, err := s.db.QueryContext(ctx, `SELECT id::text, COALESCE(name, '') FROM interfaces`)
	if err != nil {
		return names
	}
	defer rs.Close()
	for rs.Next() {
		var id, name string
		if rs.Scan(&id, &name) == nil {
			names[id] = name
		}
	}
	return names
}
