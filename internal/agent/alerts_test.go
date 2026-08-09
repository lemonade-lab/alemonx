package agent

import (
	"context"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebhookAlertSink(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusAccepted, Body: http.NoBody, Header: make(http.Header), Request: r}, nil
	})}
	if err := (WebhookAlertSink{URL: "http://127.0.0.1:1", Client: client}).Send(context.Background(), Alert{ID: "a1", Message: "test"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("webhook was not called")
	}
}

func TestAlertManagerDeduplicatesWithinPolicy(t *testing.T) {
	var count atomic.Int64
	manager := &AlertManager{Policies: map[string]AlertPolicy{"high": {RepeatInterval: time.Hour}}, Sinks: []AlertSink{countingAlertSink{count: &count}}}
	alert := Alert{Severity: "high", Kind: "incident", ProjectRoot: "/p", Fingerprint: "fp", Message: "boom"}
	manager.Notify(context.Background(), alert)
	manager.Notify(context.Background(), alert)
	time.Sleep(20 * time.Millisecond)
	if count.Load() != 1 {
		t.Fatalf("expected one deduplicated alert, got %d", count.Load())
	}
}

type failingAlertSink struct{ count *atomic.Int64 }

func (s failingAlertSink) Send(context.Context, Alert) error {
	s.count.Add(1)
	return context.DeadlineExceeded
}

func TestAlertManagerHonorsRetryPolicyAndFailureCallback(t *testing.T) {
	var attempts atomic.Int64
	var failures atomic.Int64
	manager := &AlertManager{
		Policies:          map[string]AlertPolicy{"high": {MaxRetries: 1, RetryBackoff: time.Millisecond}},
		Sinks:             []AlertSink{failingAlertSink{count: &attempts}},
		OnDeliveryFailure: func(Alert, error) { failures.Add(1) },
	}
	manager.Notify(context.Background(), Alert{Severity: "high", Kind: "incident", ProjectRoot: "/p", Fingerprint: "fp-retry", Message: "boom"})
	time.Sleep(30 * time.Millisecond)
	if attempts.Load() != 2 || failures.Load() != 1 {
		t.Fatalf("attempts=%d failures=%d", attempts.Load(), failures.Load())
	}
}

func TestAlertQueuePersistsAndClaimsDueDelivery(t *testing.T) {
	queue := NewOpsStoreAt(t.TempDir())
	delivery := AlertDelivery{ID: "delivery-1", Alert: Alert{ID: "a1", Message: "boom"}, Status: "failed", NextAttempt: time.Now().Add(-time.Second)}
	if err := queue.Enqueue(delivery); err != nil {
		t.Fatal(err)
	}
	items, err := queue.ClaimDue(context.Background(), 10)
	if err != nil || len(items) != 1 || items[0].Status != "sending" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if err := queue.Fail("delivery-1", time.Now().Add(time.Minute), "temporary"); err != nil {
		t.Fatal(err)
	}
	if err := queue.Ack("delivery-1"); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteAlertQueuePersistsDelivery(t *testing.T) {
	repo, err := NewSQLiteOpsRepository(filepath.Join(t.TempDir(), "ops.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Enqueue(AlertDelivery{ID: "sqlite-delivery", Alert: Alert{ID: "a2", Message: "boom"}, NextAttempt: time.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ClaimDue(context.Background(), 1)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if err := repo.Ack(items[0].ID); err != nil {
		t.Fatal(err)
	}
}

type countingAlertSink struct{ count *atomic.Int64 }

func (s countingAlertSink) Send(context.Context, Alert) error { s.count.Add(1); return nil }

func TestConsumeBudgetIsAtomic(t *testing.T) {
	store := NewOpsStoreAt(t.TempDir())
	if err := store.SavePolicy(OpsPolicy{ProjectRoot: "/project", Mode: "auto", AutoAllowed: true, TokenBudget: 10, MaxPM2Actions: 1, FailureCircuitBreak: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConsumeBudget("/project", 11, 0, 0); err == nil {
		t.Fatal("budget overrun must fail")
	}
	budget, err := store.GetBudget("/project")
	if err != nil {
		t.Fatal(err)
	}
	if budget.UsedTokens != 0 {
		t.Fatalf("failed consume must not mutate: %+v", budget)
	}
}
