package robot

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type RobotDiagnostics struct {
	ObservedAt         time.Time             `json:"observedAt"`
	Project            *RuntimeOverview      `json:"project,omitempty"`
	Preflight          *RuntimePreflight     `json:"preflight,omitempty"`
	LocalRuntime       *DiagnosticOperation  `json:"localRuntime,omitempty"`
	PM2                DiagnosticPM2         `json:"pm2"`
	PlatformConnection DiagnosticSignal      `json:"platformConnection"`
	Ports              []DiagnosticPort      `json:"ports"`
	Operations         []DiagnosticOperation `json:"recentOperations"`
	Findings           []DiagnosticFinding   `json:"findings"`
}

type DiagnosticOperation struct {
	ID         string     `json:"id"`
	Account    string     `json:"account,omitempty"`
	Source     string     `json:"source,omitempty"`
	Root       string     `json:"root,omitempty"`
	Action     string     `json:"action"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Progress   int        `json:"progress"`
	Error      string     `json:"error,omitempty"`
}

type DiagnosticFinding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	TaskID  string `json:"taskId,omitempty"`
}

type DiagnosticPM2 struct {
	State     string                 `json:"state"`
	Processes []DiagnosticPM2Process `json:"processes"`
}

type DiagnosticPM2Process struct {
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	PID       int        `json:"pid"`
	Memory    int64      `json:"memoryBytes"`
	CPU       float64    `json:"cpuPercent"`
	StartedAt *time.Time `json:"startedAt,omitempty"`
	Restarts  int        `json:"restarts"`
}

type DiagnosticSignal struct {
	State    string `json:"state"`
	Evidence string `json:"evidence"`
}

type DiagnosticPort struct {
	RobotPort
	ConfiguredPort       int              `json:"configuredPort,omitempty"`
	ActualPort           int              `json:"actualPort,omitempty"`
	Source               string           `json:"source"`
	Listening            bool             `json:"loopbackListening"`
	TCP                  DiagnosticSignal `json:"tcp"`
	Ownership            DiagnosticSignal `json:"ownership"`
	ApplicationReadiness DiagnosticSignal `json:"applicationReadiness"`
	HTTPStatus           int              `json:"httpStatus,omitempty"`
}

func (p *DiagnosticPort) ObserveTCP(ctx context.Context) {
	p.TCP = DiagnosticSignal{State: "unknown", Evidence: "tcp_probe_inconclusive"}
	p.Ownership = DiagnosticSignal{State: "unknown", Evidence: "listener_owner_unverified"}
	p.ApplicationReadiness = DiagnosticSignal{State: "unknown", Evidence: "application_response_unverified"}
	dialer := net.Dialer{Timeout: 200 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)))
	if err == nil {
		_ = conn.Close()
		p.Listening = true
		p.TCP = DiagnosticSignal{State: "reachable", Evidence: "loopback_tcp_connected"}
	} else if errors.Is(err, syscall.ECONNREFUSED) {
		p.TCP = DiagnosticSignal{State: "unreachable", Evidence: "loopback_connection_refused"}
	}
}

func (m Manager) DiagnoseRobot(ctx context.Context, root string, observePorts func(context.Context, []DiagnosticPM2Process) ([]DiagnosticPort, error)) RobotDiagnostics {
	result := RobotDiagnostics{
		ObservedAt: time.Now().UTC(), PlatformConnection: DiagnosticSignal{State: "unknown", Evidence: "platform_session_not_observed"}, Ports: []DiagnosticPort{},
		Operations: []DiagnosticOperation{}, Findings: []DiagnosticFinding{},
	}
	add := func(code, message string) {
		result.Findings = append(result.Findings, DiagnosticFinding{Code: code, Message: RedactRuntimeLog(message)})
	}
	if project, err := m.RuntimeOverview(root); err != nil {
		add("project_unavailable", err.Error())
	} else {
		result.Project = &project
	}
	if preflight, err := m.RuntimePreflight(root); err != nil {
		add("preflight_unavailable", err.Error())
	} else {
		result.Preflight = &preflight
		if len(preflight.Missing) > 0 {
			add("preflight_missing", "运行预检发现缺项，请查看 preflight.missing。")
		}
	}
	pm2, err := m.DiagnosticPM2Status(ctx, root)
	result.PM2 = pm2
	if err != nil {
		add("pm2_unavailable", err.Error())
	}
	for _, process := range result.PM2.Processes {
		if process.Status == "errored" || process.Status == "stopped" {
			add("pm2_"+process.Status, fmt.Sprintf("PM2 进程 %s 当前状态为 %s。", process.Name, process.Status))
		}
	}
	if observePorts == nil {
		observePorts = func(ctx context.Context, _ []DiagnosticPM2Process) ([]DiagnosticPort, error) {
			ports, err := m.Ports(root)
			if err != nil {
				return nil, err
			}
			reports := make([]DiagnosticPort, 0, len(ports))
			for _, port := range ports {
				report := DiagnosticPort{RobotPort: port, ConfiguredPort: port.Port, Source: "config"}
				report.ObserveTCP(ctx)
				reports = append(reports, report)
			}
			return reports, nil
		}
	}
	ports, err := observePorts(ctx, result.PM2.Processes)
	if err != nil {
		add("ports_unavailable", err.Error())
	}
	if ports != nil {
		result.Ports = ports
	}
	return result
}

// DiagnosticPM2Status reads current process data without starting the daemon.
func (Manager) DiagnosticPM2Status(ctx context.Context, root string) (DiagnosticPM2, error) {
	result := DiagnosticPM2{State: "unavailable", Processes: []DiagnosticPM2Process{}}
	path, err := projectPath(root)
	if err != nil {
		return result, err
	}
	processes, err := observePM2Daemon(ctx, path)
	if errors.Is(err, errPM2Offline) {
		result.State = "daemon_offline"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.State = "unregistered"
	for _, process := range processes {
		if !sameWorkspacePath(process.CWD, path) {
			continue
		}
		result.State = "registered"
		entry := DiagnosticPM2Process{ID: process.ID, Name: BoundedDiagnosticText(process.Name, 256),
			Status: BoundedDiagnosticText(process.Status, 64), PID: process.PID,
			Memory: process.Memory, CPU: process.CPU, Restarts: process.Restarts}
		if process.Uptime > 0 {
			started := time.UnixMilli(process.Uptime).UTC()
			entry.StartedAt = &started
		}
		result.Processes = append(result.Processes, entry)
	}
	return result, nil
}

// RecentDiagnosticOperations sorts the supplied snapshot, then returns a
// bounded newest-first view filtered by status.
func RecentDiagnosticOperations(operations []DiagnosticOperation, status string, limit int) []DiagnosticOperation {
	sort.SliceStable(operations, func(i, j int) bool { return operations[i].CreatedAt.After(operations[j].CreatedAt) })
	selected := make([]DiagnosticOperation, 0, min(len(operations), limit))
	for _, task := range operations {
		if status != "" && task.Status != status {
			continue
		}
		if len(selected) == limit {
			break
		}
		selected = append(selected, task)
	}
	return selected
}

func (report *RobotDiagnostics) SetRecentOperations(operations []DiagnosticOperation) {
	report.Operations = operations
	for _, task := range operations {
		if task.Status == "failed" {
			report.Findings = append(report.Findings, DiagnosticFinding{
				Code: "recent_operation_failed", Message: "近期有失败任务，请按 taskId 查询详细步骤和输出。", TaskID: task.ID,
			})
			break
		}
	}
}

func BoundedDiagnosticText(text string, maxBytes int) string {
	if len(text) > 2*maxBytes {
		// Drop the partial final line before redaction. Its secret value
		// could otherwise end outside the inspection window.
		cut := strings.LastIndexByte(text[:2*maxBytes], '\n')
		if cut < 0 {
			return "…"
		}
		text = text[:cut] + "…"
	}
	text = RedactRuntimeLog(text)
	if len(text) <= maxBytes {
		return text
	}
	return text[:maxBytes] + "…"
}
