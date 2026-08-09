package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestAlertDeliveryWorkerDrainsQueueAndReleasesLease(t *testing.T) {
	store := NewOpsStoreAt(t.TempDir())
	var sent atomic.Int64
	manager := &AlertManager{Queue: store, Sinks: []AlertSink{countingAlertSink{count: &sent}}}
	worker := &AlertDeliveryWorker{Manager: manager, Lease: NewLeaseManager(store), LeaseOwner: "one", Interval: 5 * time.Millisecond}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	manager.Notify(context.Background(), Alert{ID: "a1", Severity: "high", Message: "boom"})
	deadline := time.Now().Add(time.Second)
	for sent.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sent.Load() != 1 {
		t.Fatalf("delivery count = %d", sent.Load())
	}
	if err := worker.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := worker.Stop(); err != nil {
		t.Fatal(err)
	}
	lease, err := store.GetLease("alert-delivery")
	if err != nil || !lease.ExpiresAt.Before(time.Now()) {
		t.Fatalf("worker lease should be released: %+v err=%v", lease, err)
	}
}

func TestAlertDeliveryWorkerLeaseAllowsOneConsumer(t *testing.T) {
	store := NewOpsStoreAt(t.TempDir())
	manager := &AlertManager{Queue: store}
	first := &AlertDeliveryWorker{Manager: manager, Lease: NewLeaseManager(store), LeaseOwner: "one"}
	second := &AlertDeliveryWorker{Manager: manager, Lease: NewLeaseManager(store), LeaseOwner: "two"}
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer first.Stop()
	if err := second.Start(context.Background()); err == nil {
		t.Fatal("second worker must not acquire the active delivery lease")
	}
}
