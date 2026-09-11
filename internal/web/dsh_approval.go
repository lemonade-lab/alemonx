package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"alemonx/internal/dsh"
)

const dshApprovalTimeout = 5 * time.Minute

// dshApprovalDTO is intentionally a small browser-safe preview. It never
// contains a command, file contents, tool arguments, or provider output.
type dshApprovalDTO struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Action    string `json:"action"`
	Summary   string `json:"summary"`
	ExpiresAt string `json:"expiresAt"`
}

type dshApprovalDecision struct{ approved bool }

type pendingDSHApproval struct {
	root     string
	decision chan dshApprovalDecision
}

// dshApprovalManager gives every write action a new, single-use decision.
// Removing an entry before delivering it makes retrying an approval ID
// impossible, including after a browser reconnect.
type dshApprovalManager struct {
	mu   sync.Mutex
	pend map[string]pendingDSHApproval
}

func newDSHApprovalManager() *dshApprovalManager {
	return &dshApprovalManager{pend: map[string]pendingDSHApproval{}}
}

func (m *dshApprovalManager) register(root, sessionID, action, summary string) (dshApprovalDTO, <-chan dshApprovalDecision, func(), error) {
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		return dshApprovalDTO{}, nil, nil, err
	}
	id := base64.RawURLEncoding.EncodeToString(bytes)
	expires := time.Now().Add(dshApprovalTimeout).UTC()
	decision := make(chan dshApprovalDecision, 1)
	m.mu.Lock()
	m.pend[id] = pendingDSHApproval{root: root, decision: decision}
	m.mu.Unlock()
	approval := dshApprovalDTO{ID: id, SessionID: sessionID, Action: action, Summary: summary, ExpiresAt: expires.Format(time.RFC3339Nano)}
	return approval, decision, func() { m.remove(id) }, nil
}

func (m *dshApprovalManager) remove(id string) {
	m.mu.Lock()
	delete(m.pend, id)
	m.mu.Unlock()
}

func (m *dshApprovalManager) resolve(root, id string, approved bool) bool {
	m.mu.Lock()
	pending, ok := m.pend[id]
	if ok && pending.root == root {
		delete(m.pend, id)
	} else {
		ok = false
	}
	m.mu.Unlock()
	if !ok {
		return false
	}
	pending.decision <- dshApprovalDecision{approved: approved}
	return true
}

func (m *dshApprovalManager) cancelAll() {
	m.mu.Lock()
	pending := m.pend
	m.pend = map[string]pendingDSHApproval{}
	m.mu.Unlock()
	for _, item := range pending {
		item.decision <- dshApprovalDecision{approved: false}
	}
}

// awaitDSHApproval is the only intended write gate for the DSH loopback
// bridge. A caller must publish the returned preview to the selected session
// and block until this returns; no default approval exists.
func (s *server) awaitDSHApproval(ctx context.Context, runtime *dsh.Runtime, runtimeID, root, sessionID, action, summary string) error {
	if s.dshApprovals == nil {
		return errors.New("DSH 写操作审批不可用")
	}
	approval, decisions, cleanup, err := s.dshApprovals.register(root, sessionID, action, summary)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := runtime.PublishEvent("alemonx.approval", map[string]any{
		"type":      "approval",
		"sessionId": sessionID,
		"runtimeId": runtimeID,
		"approval":  approval,
	}); err != nil {
		return err
	}
	timer := time.NewTimer(dshApprovalTimeout)
	defer timer.Stop()
	select {
	case decision := <-decisions:
		if decision.approved {
			return nil
		}
		return errors.New("用户拒绝了该操作")
	case <-ctx.Done():
		return errors.New("审批等待已取消")
	case <-timer.C:
		return errors.New("审批已过期")
	}
}
