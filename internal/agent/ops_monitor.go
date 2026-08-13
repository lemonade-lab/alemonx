package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

type OpsLogSource func(context.Context) ([]ErrorEvent, error)
type OpsStreamSource func(context.Context, func(ErrorEvent)) error
type OpsSignalSource func(context.Context) ([]OpsSignal, error)
type IncidentHandler func(Incident, bool)

type LogCursorStore interface {
	SaveLogCursor(LogCursor) error
	GetLogCursor(string, string) (LogCursor, error)
}

type LogBatch struct {
	ProjectRoot string
	ProcessName string
	LogPath     string
	Device      int64
	Inode       uint64
	Offset      int64
	Lines       []string
	BytesRead   int64
	Rotated     bool
}

type LogBatchSource interface {
	ReadBatch(context.Context, string, string, LogCursor) (LogBatch, error)
}

// OpsMonitor is a process-local polling loop. PM2 integration supplies a log
// source, while aggregation and persistence remain independent and testable.
type OpsMonitor struct {
	Source      OpsLogSource
	Stream      OpsStreamSource
	Signals     OpsSignalSource
	Aggregator  *IncidentAggregator
	CursorStore LogCursorStore
	BatchSource LogBatchSource
	BatchRoots  []string
	// BatchRootsFn is evaluated for every poll so enabling or disabling a
	// project takes effect without restarting the workbench.
	BatchRootsFn    func() []string
	BatchProcess    func(string) []string
	Lease           LeaseManager
	LeaseKey        string
	LeaseOwner      string
	LeaseTTL        time.Duration
	OnIncident      IncidentHandler
	OnPoll          func()
	OnSignal        func(OpsSignal)
	AcquireLease    func() (func(), error)
	RenewLease      func() error
	Interval        time.Duration
	mu              sync.Mutex
	cancel          context.CancelFunc
	done            chan struct{}
	releaseLease    func()
	seen            map[string]time.Time
	fallbackStarted bool
	fallbackCancel  context.CancelFunc
	lastBatchProbe  time.Time
}

func (m *OpsMonitor) batchRoots() []string {
	if m.BatchRootsFn != nil {
		return m.BatchRootsFn()
	}
	return m.BatchRoots
}

func (m *OpsMonitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return nil
	}
	if m.Interval <= 0 {
		m.Interval = 5 * time.Second
	}
	if (m.Source == nil && m.Stream == nil && m.BatchSource == nil) || m.Aggregator == nil {
		m.mu.Unlock()
		return nil
	}
	if m.Lease != nil {
		if m.LeaseKey == "" {
			m.LeaseKey = "ops-monitor"
		}
		if m.LeaseTTL <= 0 {
			m.LeaseTTL = 45 * time.Second
		}
		if err := m.Lease.Acquire(loopContextOrBackground(ctx), m.LeaseKey, m.LeaseOwner, m.LeaseTTL); err != nil {
			m.mu.Unlock()
			return err
		}
	} else if m.AcquireLease != nil {
		release, err := m.AcquireLease()
		if err != nil {
			m.mu.Unlock()
			return err
		}
		m.releaseLease = release
	}
	loopCtx, cancel := context.WithCancel(ctx)
	m.cancel, m.done = cancel, make(chan struct{})
	if m.seen == nil {
		m.seen = map[string]time.Time{}
	}
	done := m.done
	m.mu.Unlock()
	go func() {
		defer close(done)
		defer cancel()
		// BatchSource is the primary path. Start the legacy stream only when
		// no batch source is configured; a batch read failure starts it lazily
		// via startFallback so both paths never consume the same log at once.
		if m.Stream != nil && m.BatchSource == nil {
			go m.stream(loopCtx)
		}
		ticker := time.NewTicker(m.Interval)
		defer ticker.Stop()
		m.poll(loopCtx)
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				if m.Lease != nil {
					if err := m.Lease.Renew(loopCtx, m.LeaseKey, m.LeaseOwner, m.LeaseTTL); err != nil {
						return
					}
				} else if m.RenewLease != nil {
					_ = m.RenewLease()
				}
				m.poll(loopCtx)
			}
		}
	}()
	return nil
}

func (m *OpsMonitor) poll(ctx context.Context) {
	defer func() {
		if m.OnPoll != nil {
			m.OnPoll()
		}
	}()
	if m.Source != nil {
		events, err := m.Source(ctx)
		if err == nil {
			m.ingestEvents(events)
		}
	}
	m.mu.Lock()
	fallbackActive := m.fallbackStarted
	m.mu.Unlock()
	if fallbackActive && m.BatchSource != nil {
		m.tryRestoreBatch(ctx)
		m.mu.Lock()
		fallbackActive = m.fallbackStarted
		m.mu.Unlock()
	}
	if m.BatchSource != nil && !fallbackActive {
		for _, root := range m.batchRoots() {
			processes := []string{"pm2"}
			if m.BatchProcess != nil && len(m.BatchProcess(root)) > 0 {
				processes = m.BatchProcess(root)
			}
			for _, process := range processes {
				if err := m.ingestBatch(ctx, root, process); err != nil && m.Stream != nil {
					m.startFallback(ctx)
				}
			}
		}
	}
	if m.Signals != nil {
		if signals, signalErr := m.Signals(ctx); signalErr == nil {
			for _, signal := range signals {
				keyBytes := sha256.Sum256([]byte("signal\x00" + signal.ProjectRoot + "\x00" + signal.ProcessName + "\x00" + signal.Kind + "\x00" + signal.Status + "\x00" + signal.Message))
				duplicate := false
				if m.Aggregator.store != nil {
					if persisted, markErr := m.Aggregator.store.MarkEventSeen(hex.EncodeToString(keyBytes[:])); markErr == nil {
						duplicate = persisted
					}
				}
				if !duplicate && m.OnSignal != nil {
					m.OnSignal(signal)
				}
			}
		}
	}
}

// tryRestoreBatch probes metadata reads without ingesting lines while the
// stream fallback owns consumption. A successful probe cancels the fallback;
// the following poll resumes normal file-offset ingestion.
func (m *OpsMonitor) tryRestoreBatch(ctx context.Context) {
	m.mu.Lock()
	if time.Since(m.lastBatchProbe) < 5*time.Minute {
		m.mu.Unlock()
		return
	}
	m.lastBatchProbe = time.Now()
	m.mu.Unlock()
	for _, root := range m.batchRoots() {
		processes := []string{"pm2"}
		if m.BatchProcess != nil && len(m.BatchProcess(root)) > 0 {
			processes = m.BatchProcess(root)
		}
		for _, process := range processes {
			var cursor LogCursor
			if m.CursorStore != nil {
				cursor, _ = m.CursorStore.GetLogCursor(root, process)
			}
			if _, err := m.BatchSource.ReadBatch(ctx, root, process, cursor); err != nil {
				return
			}
		}
	}
	m.mu.Lock()
	if m.fallbackStarted {
		if m.fallbackCancel != nil {
			m.fallbackCancel()
		}
		m.fallbackStarted, m.fallbackCancel = false, nil
	}
	m.mu.Unlock()
	if m.OnSignal != nil {
		m.OnSignal(OpsSignal{Kind: "log_batch", Status: "recovered", Message: "已切回 PM2 批采集", Timestamp: time.Now()})
	}
}

func (m *OpsMonitor) startFallback(ctx context.Context) {
	m.mu.Lock()
	if m.fallbackStarted || m.Stream == nil {
		m.mu.Unlock()
		return
	}
	m.fallbackStarted = true
	fallbackCtx, fallbackCancel := context.WithCancel(ctx)
	m.fallbackCancel = fallbackCancel
	m.mu.Unlock()
	if m.CursorStore != nil {
		for _, root := range m.batchRoots() {
			processes := []string{"pm2"}
			if m.BatchProcess != nil && len(m.BatchProcess(root)) > 0 {
				processes = m.BatchProcess(root)
			}
			for _, process := range processes {
				cursor, _ := m.CursorStore.GetLogCursor(root, process)
				cursor.ProjectRoot, cursor.ProcessName = root, process
				cursor.Mode, cursor.LastError, cursor.Updated = "fallback", "batch source unavailable", time.Now()
				_ = m.CursorStore.SaveLogCursor(cursor)
			}
		}
	}
	if m.OnSignal != nil {
		m.OnSignal(OpsSignal{Kind: "log_batch", Status: "fallback", Message: "PM2 文件批采集失败，已切换流式回退", Timestamp: time.Now()})
	}
	go m.stream(fallbackCtx)
}

func (m *OpsMonitor) ingestBatch(ctx context.Context, root, process string) error {
	var cursor LogCursor
	if m.CursorStore != nil {
		cursor, _ = m.CursorStore.GetLogCursor(root, process)
	}
	batch, err := m.BatchSource.ReadBatch(ctx, root, process, cursor)
	if err != nil {
		cursor.ProjectRoot, cursor.ProcessName = root, process
		cursor.Mode, cursor.LastError, cursor.Updated = "error", err.Error(), time.Now()
		if m.CursorStore != nil {
			_ = m.CursorStore.SaveLogCursor(cursor)
		}
		if m.OnSignal != nil {
			m.OnSignal(OpsSignal{ProjectRoot: root, ProcessName: process, Kind: "log_batch", Status: "error", Message: err.Error(), Timestamp: time.Now()})
		}
		return err
	}
	for _, line := range batch.Lines {
		m.ingestEvents(ParsePM2LogOutput(root, process, line, time.Now()))
	}
	if m.CursorStore != nil {
		if batch.Rotated || batch.Offset < cursor.Offset {
			cursor.Offset = 0
		}
		cursor.ProjectRoot, cursor.ProcessName = root, process
		cursor.LogPath, cursor.Device, cursor.Inode = batch.LogPath, batch.Device, batch.Inode
		cursor.Offset = batch.Offset
		cursor.BytesRead += batch.BytesRead
		cursor.File = FileLogCursor{LogPath: batch.LogPath, Device: batch.Device, Inode: batch.Inode, Offset: batch.Offset, BytesRead: cursor.File.BytesRead + batch.BytesRead, Rotations: cursor.File.Rotations}
		if batch.Rotated {
			cursor.Rotations++
			cursor.File.Rotations++
		}
		cursor.Mode, cursor.LastError = "batch", ""
		cursor.Updated = time.Now()
		return m.CursorStore.SaveLogCursor(cursor)
	}
	return nil
}

func (m *OpsMonitor) ingestEvents(events []ErrorEvent) {
	cursorHashes := map[string]string{}
	cursorCounts := map[string]int64{}
	for _, event := range events {
		cursorKey := event.ProjectRoot + "\x00" + event.ProcessName
		cursorHashes[cursorKey] = event.Fingerprint + "\x00" + event.Normalized
		// Stream sources expose complete log lines. Track their encoded byte
		// length rather than an event count so a file-backed source can resume
		// from a meaningful offset; inode/device metadata is preserved by the
		// source when available.
		cursorCounts[cursorKey] += int64(len([]byte(event.RawMessage)) + 1)
		keyBytes := sha256.Sum256([]byte(event.ProjectRoot + "\x00" + event.ProcessName + "\x00" + event.Normalized + "\x00" + event.RawMessage))
		key := hex.EncodeToString(keyBytes[:])
		duplicate := false
		if m.Aggregator.store != nil {
			if persisted, markErr := m.Aggregator.store.MarkEventSeen(key); markErr == nil {
				duplicate = persisted
			}
		} else {
			m.mu.Lock()
			_, duplicate = m.seen[key]
			m.seen[key] = time.Now()
			m.mu.Unlock()
		}
		if duplicate {
			continue
		}
		incident, fresh, ingestErr := m.Aggregator.Ingest(event)
		if ingestErr == nil && m.OnIncident != nil && (fresh || incident.Status == IncidentObserving) {
			m.OnIncident(incident, true)
		}
	}
	if m.CursorStore != nil {
		for key, hash := range cursorHashes {
			parts := strings.SplitN(key, "\x00", 2)
			if len(parts) != 2 {
				continue
			}
			previous, _ := m.CursorStore.GetLogCursor(parts[0], parts[1])
			previous.ProjectRoot, previous.ProcessName = parts[0], parts[1]
			previous.WindowHash, previous.Updated = hash, time.Now()
			previous.Stream.WindowHash = hash
			previous.Stream.Events += cursorCounts[key]
			previous.Stream.Updated = previous.Updated
			_ = m.CursorStore.SaveLogCursor(previous)
		}
	}
}

func (m *OpsMonitor) stream(ctx context.Context) {
	delay := time.Second
	for ctx.Err() == nil {
		err := m.Stream(ctx, func(event ErrorEvent) { m.ingestEvents([]ErrorEvent{event}) })
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			delay = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		}
	}
}

func (m *OpsMonitor) Stop() error {
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	fallbackCancel := m.fallbackCancel
	m.cancel, m.done = nil, nil
	m.fallbackCancel, m.fallbackStarted = nil, false
	release := m.releaseLease
	m.releaseLease = nil
	m.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	if fallbackCancel != nil {
		fallbackCancel()
	}
	if done != nil {
		<-done
	}
	if m.Lease != nil {
		_ = m.Lease.Release(context.Background(), m.LeaseKey, m.LeaseOwner)
	} else if release != nil {
		release()
	}
	return nil
}

func loopContextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
