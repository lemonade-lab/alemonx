package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

type MetricsRepository interface {
	Increment(name, root, fingerprint string, value int64) error
	Observe(name, root string, value float64) error
	Snapshot(root string) (OpsMetrics, error)
	Query(root string, from, to time.Time) ([]MetricPoint, error)
}

type MetricPoint struct {
	Name        string    `json:"name"`
	Root        string    `json:"root"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Value       float64   `json:"value"`
	Updated     time.Time `json:"updated"`
	WindowStart time.Time `json:"windowStart"`
}

type metricValue struct {
	Name    string    `json:"name"`
	Root    string    `json:"root"`
	FP      string    `json:"fingerprint"`
	Value   float64   `json:"value"`
	Updated time.Time `json:"updated"`
	Bucket  time.Time `json:"bucket"`
}

const metricBucket = 5 * time.Minute
const metricRetention = 90 * 24 * time.Hour

var metricsMu sync.Mutex

func (s *OpsStore) Increment(name, root, fingerprint string, value int64) error {
	return s.updateMetric(name, root, fingerprint, float64(value))
}

func (s *OpsStore) Observe(name, root string, value float64) error {
	return s.updateMetric(name, root, "", value)
}

func (s *OpsStore) updateMetric(name, root, fingerprint string, value float64) error {
	metricsMu.Lock()
	defer metricsMu.Unlock()
	path := filepath.Join(s.dir, "metrics.json")
	var items []metricValue
	if err := readJSONFile(path, &items); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	now := time.Now().UTC()
	bucket := now.Truncate(metricBucket)
	found := false
	for i := range items {
		if items[i].Name == name && items[i].Root == root && items[i].FP == fingerprint && items[i].Bucket.Equal(bucket) {
			items[i].Value += value
			items[i].Updated = now
			found = true
			break
		}
	}
	if !found {
		items = append(items, metricValue{Name: name, Root: root, FP: fingerprint, Value: value, Updated: now, Bucket: bucket})
	}
	cutoff := now.Add(-metricRetention)
	items = slices.DeleteFunc(items, func(item metricValue) bool { return !item.Bucket.IsZero() && item.Bucket.Before(cutoff) })
	return atomicJSONFile(path, items)
}

func (s *OpsStore) Snapshot(root string) (OpsMetrics, error) {
	var items []metricValue
	if err := readJSONFile(filepath.Join(s.dir, "metrics.json"), &items); err != nil && !errors.Is(err, os.ErrNotExist) {
		return OpsMetrics{}, err
	}
	return snapshotMetrics(items, root), nil
}

func (s *OpsStore) Query(root string, from, to time.Time) ([]MetricPoint, error) {
	var items []metricValue
	if err := readJSONFile(filepath.Join(s.dir, "metrics.json"), &items); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	out := make([]MetricPoint, 0, len(items))
	for _, item := range items {
		at := item.Bucket
		if at.IsZero() {
			at = item.Updated
		}
		if root != "" && item.Root != root || !from.IsZero() && at.Add(metricBucket).Before(from) || !to.IsZero() && at.After(to) {
			continue
		}
		out = append(out, MetricPoint{Name: item.Name, Root: item.Root, Fingerprint: item.FP, Value: item.Value, Updated: item.Updated, WindowStart: item.Bucket})
	}
	return out, nil
}

func snapshotMetrics(items []metricValue, root string) OpsMetrics {
	var out OpsMetrics
	for _, item := range items {
		if root != "" && item.Root != root {
			continue
		}
		n := item.Name
		switch n {
		case "incident_total":
			out.Incidents += int(item.Value)
		case "incident_deduplicated_total":
			out.IncidentDeduplicated += int(item.Value)
		case "ai_wakeup_total":
			out.AIWakeups += int(item.Value)
		case "maintenance_success_total":
			out.AutoFixSuccess += int(item.Value)
		case "maintenance_failure_total":
			out.MaintenanceFailures += int(item.Value)
		case "maintenance_rollback_total":
			out.Rollbacks += int(item.Value)
		case "pm2_action_failure_total":
			out.PM2ActionFailures += int(item.Value)
		case "budget_exhausted_total":
			out.BudgetExhausted += int(item.Value)
		case "lease_takeover_total":
			out.LeaseTakeovers += int(item.Value)
		case "recovery_conflict_total":
			out.RecoveryConflicts += int(item.Value)
		case "alert_delivery_total":
			out.AlertDeliveryTotal += int(item.Value)
		case "alert_delivery_failure_total":
			out.AlertDeliveryFailures += int(item.Value)
		case "incident_mttr_seconds":
			out.AverageRecoverySecs += item.Value
		}
	}
	return out
}

func (s *SQLiteOpsRepository) Increment(name, root, fingerprint string, value int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	bucket := now.Truncate(metricBucket)
	_, err := s.db.Exec(`INSERT INTO ops_metric_buckets(metric_name,project_root,fingerprint,bucket_start,value,updated) VALUES(?,?,?,?,?,?) ON CONFLICT(metric_name,project_root,fingerprint,bucket_start) DO UPDATE SET value=value+excluded.value,updated=excluded.updated`, name, root, fingerprint, bucket.Format(time.RFC3339Nano), value, now.Format(time.RFC3339Nano))
	if err == nil {
		_, _ = s.db.Exec(`DELETE FROM ops_metric_buckets WHERE bucket_start<?`, now.Add(-metricRetention).Format(time.RFC3339Nano))
	}
	return err
}

func (s *SQLiteOpsRepository) Observe(name, root string, value float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	bucket := now.Truncate(metricBucket)
	_, err := s.db.Exec(`INSERT INTO ops_metric_buckets(metric_name,project_root,fingerprint,bucket_start,value,updated) VALUES(?,?,?,?,?,?) ON CONFLICT(metric_name,project_root,fingerprint,bucket_start) DO UPDATE SET value=value+excluded.value,updated=excluded.updated`, name, root, "", bucket.Format(time.RFC3339Nano), value, now.Format(time.RFC3339Nano))
	if err == nil {
		_, _ = s.db.Exec(`DELETE FROM ops_metric_buckets WHERE bucket_start<?`, now.Add(-metricRetention).Format(time.RFC3339Nano))
	}
	return err
}

func (s *SQLiteOpsRepository) Snapshot(root string) (OpsMetrics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT metric_name,SUM(value) FROM ops_metric_buckets WHERE project_root=? OR ?='' GROUP BY metric_name`, root, root)
	if err != nil {
		return OpsMetrics{}, err
	}
	defer rows.Close()
	var items []metricValue
	for rows.Next() {
		var name string
		var value float64
		if err := rows.Scan(&name, &value); err != nil {
			return OpsMetrics{}, err
		}
		items = append(items, metricValue{Name: name, Root: root, Value: value})
	}
	return snapshotMetrics(items, root), rows.Err()
}

func (s *SQLiteOpsRepository) Query(root string, from, to time.Time) ([]MetricPoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query := `SELECT metric_name,project_root,fingerprint,value,bucket_start,updated FROM ops_metric_buckets WHERE (project_root=? OR ?='')`
	args := []any{root, root}
	if !from.IsZero() {
		// RFC3339Nano omits trailing fractional zeroes, so lexical comparison can
		// exclude a bucket exactly at the query boundary ("...00Z" sorts after
		// "...00.1Z"). Let SQLite compare actual timestamps instead.
		query += ` AND julianday(bucket_start)>=julianday(?)`
		args = append(args, from.UTC().Add(-metricBucket).Format(time.RFC3339Nano))
	}
	if !to.IsZero() {
		query += ` AND julianday(bucket_start)<=julianday(?)`
		args = append(args, to.UTC().Format(time.RFC3339Nano))
	}
	query += ` ORDER BY bucket_start DESC, metric_name ASC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MetricPoint
	for rows.Next() {
		var item MetricPoint
		var bucket, updated string
		if err := rows.Scan(&item.Name, &item.Root, &item.Fingerprint, &item.Value, &bucket, &updated); err != nil {
			return nil, err
		}
		item.Updated, _ = time.Parse(time.RFC3339Nano, updated)
		item.WindowStart, _ = time.Parse(time.RFC3339Nano, bucket)
		out = append(out, item)
	}
	return out, rows.Err()
}

func recordMetric(store OpsRepository, name, root, fingerprint string, value int64) {
	if repo, ok := store.(MetricsRepository); ok {
		_ = repo.Increment(name, root, fingerprint, value)
	}
}

var _ MetricsRepository = (*OpsStore)(nil)
var _ MetricsRepository = (*SQLiteOpsRepository)(nil)
