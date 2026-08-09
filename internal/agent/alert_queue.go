package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *OpsStore) Enqueue(delivery AlertDelivery) error {
	if delivery.ID == "" {
		delivery.ID = newID("delivery")
	}
	if delivery.Status == "" {
		delivery.Status = "pending"
	}
	if delivery.NextAttempt.IsZero() {
		delivery.NextAttempt = time.Now()
	}
	delivery.Updated = time.Now()
	return atomicJSONFile(filepath.Join(s.dir, "alert-delivery-"+delivery.ID+".json"), delivery)
}

func (s *OpsStore) ClaimDue(ctx context.Context, limit int) ([]AlertDelivery, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []AlertDelivery{}, nil
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var out []AlertDelivery
	for _, entry := range entries {
		if len(out) >= limit || entry.IsDir() || !strings.HasPrefix(entry.Name(), "alert-delivery-") {
			continue
		}
		var item AlertDelivery
		if readJSONFile(filepath.Join(s.dir, entry.Name()), &item) == nil && item.Status != "acked" && item.Status != "failed_permanent" && !item.NextAttempt.After(now) {
			// A second worker must not claim an in-flight delivery. Treat a stale
			// sending record as abandoned after one minute so a crashed worker
			// cannot strand an alert forever.
			if item.Status == "sending" && item.Updated.After(now.Add(-time.Minute)) {
				continue
			}
			item.Status, item.Updated = "sending", now
			if atomicJSONFile(filepath.Join(s.dir, entry.Name()), item) == nil {
				out = append(out, item)
			}
		}
	}
	return out, nil
}

func (s *OpsStore) Ack(id string) error {
	var item AlertDelivery
	path := filepath.Join(s.dir, "alert-delivery-"+id+".json")
	if err := readJSONFile(path, &item); err != nil {
		return err
	}
	item.Status, item.Updated = "acked", time.Now()
	return atomicJSONFile(path, item)
}

func (s *OpsStore) Fail(id string, nextAttempt time.Time, reason string) error {
	var item AlertDelivery
	path := filepath.Join(s.dir, "alert-delivery-"+id+".json")
	if err := readJSONFile(path, &item); err != nil {
		return err
	}
	item.Attempts++
	item.Status, item.NextAttempt, item.LastError, item.Updated = "failed", nextAttempt, reason, time.Now()
	if nextAttempt.After(time.Now().Add(50 * 365 * 24 * time.Hour)) {
		item.Status = "failed_permanent"
	}
	return atomicJSONFile(path, item)
}

func (s *SQLiteOpsRepository) Enqueue(delivery AlertDelivery) error {
	if delivery.ID == "" {
		delivery.ID = newID("delivery")
	}
	if delivery.Status == "" {
		delivery.Status = "pending"
	}
	if delivery.NextAttempt.IsZero() {
		delivery.NextAttempt = time.Now()
	}
	delivery.Updated = time.Now()
	payload, err := json.Marshal(delivery)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT OR REPLACE INTO alert_deliveries(id,payload,status,next_attempt,updated) VALUES(?,?,?,?,?)`, delivery.ID, string(payload), delivery.Status, delivery.NextAttempt.Format(time.RFC3339Nano), delivery.Updated.Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteOpsRepository) ClaimDue(ctx context.Context, limit int) ([]AlertDelivery, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT id,payload FROM alert_deliveries WHERE status IN ('pending','failed') AND next_attempt<=? OR (status='sending' AND updated<=?) ORDER BY next_attempt ASC, id ASC LIMIT ?`, now.Format(time.RFC3339Nano), now.Add(-time.Minute).Format(time.RFC3339Nano), limit)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	defer rows.Close()
	var out []AlertDelivery
	for rows.Next() {
		var id, payload string
		if err := rows.Scan(&id, &payload); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		var item AlertDelivery
		if json.Unmarshal([]byte(payload), &item) != nil {
			continue
		}
		item.Status, item.Updated = "sending", now
		updated, _ := json.Marshal(item)
		if _, err := tx.Exec(`UPDATE alert_deliveries SET payload=?,status=?,updated=? WHERE id=? AND status!='acked'`, string(updated), item.Status, now.Format(time.RFC3339Nano), id); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return out, tx.Commit()
}

func (s *SQLiteOpsRepository) Ack(id string) error {
	return s.updateDelivery(id, func(item *AlertDelivery) { item.Status, item.Updated = "acked", time.Now() })
}

func (s *SQLiteOpsRepository) Fail(id string, nextAttempt time.Time, reason string) error {
	return s.updateDelivery(id, func(item *AlertDelivery) {
		item.Attempts++
		item.Status, item.NextAttempt, item.LastError, item.Updated = "failed", nextAttempt, reason, time.Now()
		if nextAttempt.After(time.Now().Add(50 * 365 * 24 * time.Hour)) {
			item.Status = "failed_permanent"
		}
	})
}

func (s *SQLiteOpsRepository) updateDelivery(id string, update func(*AlertDelivery)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var payload string
	if err := s.db.QueryRow(`SELECT payload FROM alert_deliveries WHERE id=?`, id).Scan(&payload); err != nil {
		return err
	}
	var item AlertDelivery
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		return err
	}
	update(&item)
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE alert_deliveries SET payload=?,status=?,next_attempt=?,updated=? WHERE id=?`, string(b), item.Status, item.NextAttempt.Format(time.RFC3339Nano), item.Updated.Format(time.RFC3339Nano), id)
	return err
}

var _ AlertQueue = (*OpsStore)(nil)
var _ AlertQueue = (*SQLiteOpsRepository)(nil)
