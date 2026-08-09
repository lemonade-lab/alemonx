package agent

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Alert struct {
	ID          string    `json:"id"`
	Severity    string    `json:"severity"`
	Kind        string    `json:"kind"`
	ProjectRoot string    `json:"projectRoot"`
	IncidentID  string    `json:"incidentId,omitempty"`
	Message     string    `json:"message"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Resolved    bool      `json:"resolved"`
	Timestamp   time.Time `json:"timestamp"`
}

type AlertSink interface {
	Send(context.Context, Alert) error
}

type AlertRecordStore interface {
	SaveAlert(AlertRecord) error
	ListAlerts() ([]AlertRecord, error)
}

type AlertDelivery struct {
	ID          string    `json:"id"`
	Alert       Alert     `json:"alert"`
	Attempts    int       `json:"attempts"`
	NextAttempt time.Time `json:"nextAttempt"`
	LastError   string    `json:"lastError,omitempty"`
	Status      string    `json:"status"` // pending/sending/acked/failed
	Updated     time.Time `json:"updated"`
}

type AlertQueue interface {
	Enqueue(AlertDelivery) error
	ClaimDue(context.Context, int) ([]AlertDelivery, error)
	Ack(string) error
	Fail(string, time.Time, string) error
}

type WebhookAlertSink struct {
	URL    string
	Client *http.Client
	Secret string
}

func (s WebhookAlertSink) Send(ctx context.Context, alert Alert) error {
	if strings.TrimSpace(s.URL) == "" {
		return errors.New("Webhook URL 为空")
	}
	body, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.Secret != "" {
		mac := hmac.New(sha256.New, []byte(s.Secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-ALX-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("Webhook 返回非成功状态")
	}
	return nil
}

// AlertManager dispatches asynchronously. Record is called synchronously so
// the alert remains auditable even when every external sink is unavailable.
type AlertManager struct {
	Sinks             []AlertSink
	Record            func(Alert) error
	Policies          map[string]AlertPolicy
	mu                sync.Mutex
	lastSent          map[string]time.Time
	OnDeliveryFailure func(Alert, error)
	RetryStore        AlertRecordStore
	Queue             AlertQueue
}

// RetryPending replays delivery failures persisted by the repository. It is
// deliberately best-effort: a sink outage must never block incident or task
// execution, and the failure remains durable for the next poll.
func (m *AlertManager) RetryPending(ctx context.Context) {
	if m == nil || m.RetryStore == nil {
		return
	}
	records, err := m.RetryStore.ListAlerts()
	if err != nil {
		return
	}
	now := time.Now()
	for _, record := range records {
		if record.Status != "delivery_failed" || record.NextAttempt.After(now) {
			continue
		}
		policy := m.Policies[record.Severity]
		maxRetries := policy.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 3
		}
		if record.RetryCount >= maxRetries {
			continue
		}
		_ = m.RetryStore.SaveAlert(func() AlertRecord {
			record.Status = "open"
			record.Updated = now
			return record
		}())
		key := record.Fingerprint
		if key == "" {
			key = record.Kind + "\x00" + record.ProjectRoot + "\x00" + record.Message
		}
		m.mu.Lock()
		if m.lastSent != nil {
			delete(m.lastSent, key)
		}
		m.mu.Unlock()
		m.Notify(ctx, record.Alert)
	}
}

// RetryQueue drains durable deliveries independently from the incident path.
// It is safe to call from a periodic worker and can be interrupted by ctx.
func (m *AlertManager) RetryQueue(ctx context.Context) {
	if m == nil || m.Queue == nil {
		return
	}
	items, err := m.Queue.ClaimDue(ctx, 50)
	if err != nil {
		return
	}
	for _, item := range items {
		delivered := true
		for _, sink := range m.Sinks {
			if sink == nil {
				continue
			}
			if err := sink.Send(ctx, item.Alert); err != nil {
				delivered = false
				_ = m.Queue.Fail(item.ID, time.Now().Add(time.Minute), err.Error())
			}
		}
		if delivered {
			_ = m.Queue.Ack(item.ID)
		}
	}
}

func (m *AlertManager) Notify(ctx context.Context, alert Alert) {
	if alert.ID == "" {
		alert.ID = newID("alert")
	}
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now()
	}
	key := alert.Fingerprint
	if key == "" {
		key = alert.Kind + "\x00" + alert.ProjectRoot + "\x00" + alert.Message
	}
	interval := 5 * time.Minute
	if policy, ok := m.Policies[alert.Severity]; ok && policy.RepeatInterval > 0 {
		interval = policy.RepeatInterval
	}
	m.mu.Lock()
	if m.lastSent == nil {
		m.lastSent = map[string]time.Time{}
	}
	if !alert.Resolved && !alert.Timestamp.IsZero() && time.Since(m.lastSent[key]) < interval {
		m.mu.Unlock()
		return
	}
	m.lastSent[key] = alert.Timestamp
	m.mu.Unlock()
	if m.Record != nil {
		_ = m.Record(alert)
	}
	for _, sink := range m.Sinks {
		if sink == nil {
			continue
		}
		go func(sink AlertSink) {
			policy := AlertPolicy{MaxRetries: 3, RetryBackoff: 100 * time.Millisecond}
			if configured, ok := m.Policies[alert.Severity]; ok {
				if configured.MaxRetries >= 0 {
					policy.MaxRetries = configured.MaxRetries
				}
				if configured.RetryBackoff > 0 {
					policy.RetryBackoff = configured.RetryBackoff
				}
			}
			for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
				if err := sink.Send(ctx, alert); err == nil {
					return
				} else if attempt == policy.MaxRetries {
					if m.OnDeliveryFailure != nil {
						m.OnDeliveryFailure(alert, err)
					}
					if m.Queue != nil {
						_ = m.Queue.Enqueue(AlertDelivery{ID: "delivery-" + alert.ID, Alert: alert, Attempts: attempt + 1, NextAttempt: time.Now().Add(policy.RetryBackoff), LastError: err.Error(), Status: "failed", Updated: time.Now()})
					}
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(attempt+1) * policy.RetryBackoff):
				}
			}
		}(sink)
	}
}
