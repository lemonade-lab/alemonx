package agent

import (
	"context"
	"errors"
	"sync"
	"time"
)

// AlertDeliveryWorker is the single durable consumer for AlertQueue. It is
// intentionally process-local for the first production deployment, while the
// lease makes a second ALemonX instance passive instead of double-delivering.
type AlertDeliveryWorker struct {
	Manager     *AlertManager
	Lease       LeaseManager
	LeaseKey    string
	LeaseOwner  string
	LeaseTTL    time.Duration
	Interval    time.Duration
	OnLeaseLost func(error)

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func (w *AlertDeliveryWorker) Start(ctx context.Context) error {
	if w == nil || w.Manager == nil || w.Manager.Queue == nil {
		return errors.New("告警投递队列未初始化")
	}
	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return nil
	}
	if w.LeaseKey == "" {
		w.LeaseKey = "alert-delivery"
	}
	if w.LeaseTTL <= 0 {
		w.LeaseTTL = 45 * time.Second
	}
	if w.Interval <= 0 {
		w.Interval = 5 * time.Second
	}
	if w.Lease != nil {
		if err := w.Lease.Acquire(loopContextOrBackground(ctx), w.LeaseKey, w.LeaseOwner, w.LeaseTTL); err != nil {
			w.mu.Unlock()
			return err
		}
	}
	workerCtx, cancel := context.WithCancel(loopContextOrBackground(ctx))
	w.cancel, w.done = cancel, make(chan struct{})
	done := w.done
	w.mu.Unlock()
	go func() {
		defer close(done)
		defer cancel()
		w.Manager.RetryQueue(workerCtx)
		ticker := time.NewTicker(w.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				if w.Lease != nil {
					if err := w.Lease.Renew(workerCtx, w.LeaseKey, w.LeaseOwner, w.LeaseTTL); err != nil {
						if w.OnLeaseLost != nil {
							w.OnLeaseLost(err)
						}
						return
					}
				}
				w.Manager.RetryQueue(workerCtx)
			}
		}
	}()
	return nil
}

func (w *AlertDeliveryWorker) Stop() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	w.cancel, w.done = nil, nil
	w.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	if done != nil {
		<-done
	}
	if w.Lease != nil {
		return w.Lease.Release(context.Background(), w.LeaseKey, w.LeaseOwner)
	}
	return nil
}

func (w *AlertDeliveryWorker) Running() bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cancel != nil
}
