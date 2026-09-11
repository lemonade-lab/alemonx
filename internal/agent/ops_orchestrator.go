package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OpsOrchestrator coordinates the policy decision and the side effects. The
// web layer supplies guarded PM2/task callbacks, keeping this package safe to
// test without spawning processes or calling an AI provider.
type OpsOrchestrator struct {
	Store  OpsRepository
	Policy func(string) (OpsPolicy, error)
	// StartDiagnostic creates a DSH session which may inspect the project and
	// propose a repair. It must never make a write or PM2 change by itself.
	StartDiagnostic func(Incident, AutoFixDecision) (string, error)
	// StartFix is retained only for source compatibility with embedders. The
	// automatic-maintenance path never calls it.
	StartFix func(Incident, AutoFixDecision) (string, error)
	// PM2 is retained for source compatibility with embedders. Production
	// callers must provide PM2Guarded; the unguarded callback is never used by
	// the automatic-maintenance path.
	PM2        func(string, string) (string, error)
	PM2Guarded func(string, string, string) (string, error)
	AI         func(Incident, OpsPolicy) (AutoFixDecision, error)
	Now        func() time.Time
}

func (o *OpsOrchestrator) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *OpsOrchestrator) Analyze(id string) (Incident, AutoFixDecision, error) {
	if o == nil || o.Store == nil {
		return Incident{}, AutoFixDecision{}, errors.New("运维编排器未初始化")
	}
	incident, err := o.Store.GetIncident(id)
	if err != nil {
		return Incident{}, AutoFixDecision{}, err
	}
	policy := OpsPolicy{ProjectRoot: incident.ProjectRoot, Mode: "observe"}
	if o.Policy != nil {
		if value, policyErr := o.Policy(incident.ProjectRoot); policyErr == nil {
			policy = value
		}
	}
	decision := DecideAutoFix(incident, policy)
	if o.AI != nil && policy.Mode != "off" && policy.Mode != "observe" {
		if candidate, aiErr := o.AI(incident, policy); aiErr == nil && validOpsDecision(candidate) && decision.Risk != "high" && policy.Mode != "off" && policy.Mode != "observe" && opsDecisionCanNarrow(decision.Action, candidate.Action) && !(candidate.Action == "auto_fix" && !policy.AllowCodeChanges) {
			decision = candidate
		}
	}
	incident.Decision, incident.DecisionReason, incident.Updated = decision.Action, decision.Reason, o.now()
	if decision.Action == "create_todo" {
		incident.Status = IncidentTodo
	} else if decision.Action == "observe_only" {
		incident.Status = IncidentObserving
	} else if decision.Action == "auto_fix" || decision.Action == "restart_process" {
		incident.Status = IncidentInvestigating
	}
	if err := o.Store.SaveIncident(incident); err != nil {
		return incident, decision, err
	}
	recordMetric(o.Store, "ai_wakeup_total", incident.ProjectRoot, incident.Fingerprint, 1)
	if decision.Action == "create_todo" {
		_ = o.createTodo(incident, decision)
		return incident, decision, nil
	}
	if decision.Action == "restart_process" {
		if existing, ok := o.activeRunForIncident(incident.ID); ok {
			return incident, decision, fmt.Errorf("事件已有进行中的维护任务：%s", existing.ID)
		}
		decision.RequiresHuman = true
		decision.Reason = strings.TrimSpace(decision.Reason + "；PM2 操作须在 DSH 会话中逐次审批")
		run := MaintenanceRun{ID: fmt.Sprintf("maint-%d", o.now().UnixNano()), IncidentID: incident.ID, Decision: decision, PM2Actions: []string{"pm2-restart"}, Status: "pending_approval", ApprovalSource: "automatic", Created: o.now()}
		incident.Status, incident.Decision, incident.DecisionReason = IncidentTodo, decision.Action, decision.Reason
		_ = o.createTodo(incident, decision)
		_ = o.Store.SaveMaintenance(run)
		_ = o.Store.SaveIncident(incident)
		return incident, decision, nil
	}
	if decision.Action == "auto_fix" {
		if existing, ok := o.activeRunForIncident(incident.ID); ok {
			return incident, decision, fmt.Errorf("事件已有进行中的维护任务：%s", existing.ID)
		}
		decision.RequiresHuman = true
		decision.Reason = strings.TrimSpace(decision.Reason + "；仅创建 DSH 诊断与待审批修复计划")
		run := MaintenanceRun{ID: fmt.Sprintf("maint-%d", o.now().UnixNano()), IncidentID: incident.ID, Decision: decision, Status: "pending_approval", ApprovalSource: "automatic", Created: o.now()}
		if o.StartDiagnostic == nil {
			run.Status, run.Error = "failed", "DSH 诊断执行器未配置"
			_ = o.createTodo(incident, decision)
			incident.Status = IncidentTodo
			_ = o.Store.SaveMaintenance(run)
			_ = o.Store.SaveIncident(incident)
			return incident, decision, nil
		}
		sessionID, startErr := o.StartDiagnostic(incident, decision)
		if startErr != nil {
			run.Status, run.Error = "failed", startErr.Error()
			_ = o.createTodo(incident, decision)
		} else {
			run.DSHSessionID = sessionID
			incident.LastDSHSessionID, incident.Status = sessionID, IncidentTodo
			_ = o.createTodo(incident, decision)
		}
		_ = o.Store.SaveMaintenance(run)
		_ = o.Store.SaveIncident(incident)
	}
	return incident, decision, nil
}

func (o *OpsOrchestrator) activeRunForIncident(incidentID string) (MaintenanceRun, bool) {
	if o == nil || o.Store == nil {
		return MaintenanceRun{}, false
	}
	runs, err := o.Store.ListMaintenance()
	if err != nil {
		return MaintenanceRun{}, false
	}
	for _, run := range runs {
		switch run.Status {
		case "queued", "running", "fixing", "verifying", "observing", "pending_approval", "human_review":
			if run.IncidentID == incidentID {
				return run, true
			}
		}
	}
	return MaintenanceRun{}, false
}

func opsDecisionCanNarrow(base, candidate string) bool {
	if candidate == "create_todo" || candidate == "observe_only" || candidate == "escalate" {
		return true
	}
	return base == candidate
}

func (o *OpsOrchestrator) createTodo(incident Incident, decision AutoFixDecision) error {
	todo := OpsTodo{ID: "todo-" + incident.ID, IncidentID: incident.ID, ProjectRoot: incident.ProjectRoot, Title: "处理：" + incident.ProcessName, Summary: incident.Sample, Severity: incident.Severity, Reason: decision.Reason, SuggestedPlan: decision.Plan, Status: "open", Created: o.now(), Updated: o.now()}
	return o.Store.SaveTodo(todo)
}

func validOpsDecision(decision AutoFixDecision) bool {
	switch decision.Action {
	case "observe_only", "restart_process", "auto_fix", "create_todo", "escalate":
		return true
	default:
		return false
	}
}

func ParseAutoFixDecision(raw string) (AutoFixDecision, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		if idx := strings.IndexByte(raw, '\n'); idx >= 0 {
			raw = raw[idx+1:]
		}
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	var decision AutoFixDecision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil {
		return decision, err
	}
	if !validOpsDecision(decision) {
		return decision, errors.New("运维决策动作无效")
	}
	return decision, nil
}

func (o *OpsOrchestrator) Observe(id string) error {
	incident, err := o.Store.GetIncident(id)
	if err != nil {
		return err
	}
	if incident.Status != IncidentObserving {
		return fmt.Errorf("事件当前不在观察状态：%s", incident.Status)
	}
	window := 5 * time.Minute
	if o.Policy != nil {
		if policy, policyErr := o.Policy(incident.ProjectRoot); policyErr == nil && policy.ObservationMinutes > 0 {
			window = time.Duration(policy.ObservationMinutes) * time.Minute
		}
	}
	if o.now().Before(incident.LastSeen.Add(window)) {
		return errors.New("观察窗口尚未结束")
	}
	incident.Status, incident.Updated = IncidentResolved, o.now()
	if err := o.Store.SaveIncident(incident); err != nil {
		return err
	}
	items, _ := o.Store.ListMaintenance()
	for _, item := range items {
		if item.IncidentID == id && item.Status == "observing" {
			item.Status = "resolved"
			now := o.now()
			item.Finished = &now
			_ = o.Store.SaveMaintenance(item)
		}
	}
	return nil
}

func (o *OpsOrchestrator) Approve(id string) (Incident, error) {
	incident, err := o.Store.GetIncident(id)
	if err != nil {
		return Incident{}, err
	}
	policy := OpsPolicy{ProjectRoot: incident.ProjectRoot, Mode: "strict"}
	if o.Policy != nil {
		if current, policyErr := o.Policy(incident.ProjectRoot); policyErr == nil {
			policy = current
		}
	}
	// A one-time approval may bypass the mode's human gate, never the project
	// whitelist, high-risk classifier or mandatory verification command.
	if policy.Mode == "strict" || policy.Mode == "observe" {
		policy.Mode = "auto"
		policy.SingleApproval = true
	}
	decision := DecideAutoFix(incident, policy)
	if decision.Action == "create_todo" {
		return incident, errors.New("该事件仍被安全策略阻止")
	}
	// This only transfers the suggestion to a human review. The actual DSH
	// tool call creates its own one-time approval and this endpoint must not
	// resurrect the legacy automatic executor.
	runs, _ := o.Store.ListMaintenance()
	for _, run := range runs {
		if run.IncidentID == incident.ID && run.Status == "pending_approval" {
			run.Status, run.ApprovalSource = "human_review", "human"
			_ = o.Store.SaveMaintenance(run)
		}
	}
	incident.Status, incident.Decision, incident.Updated = IncidentTodo, "human_review", o.now()
	if err := o.Store.SaveIncident(incident); err != nil {
		return incident, err
	}
	return incident, nil
}

func IsAllowedPM2Action(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "pm2", "pm2-status", "pm2-logs", "pm2-stop", "pm2-restart", "pm2-reload", "pm2-delete":
		return true
	default:
		return false
	}
}
