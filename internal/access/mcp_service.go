package access

import "errors"

var ErrMCPServiceControl = errors.New("只有超级管理员可以启动或停止 MCP。")

func (m *Manager) MCPAutoStart() (bool, error) {
	data, err := m.current()
	return data.MCPAutoStart, err
}

func (m *Manager) SetMCPAutoStart(session string, identity Status, enabled bool) error {
	unlock, err := m.lockWrite()
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.reload(); err != nil {
		return err
	}
	status := m.sessionStatus(m.data, session)
	if status.Enabled != identity.Enabled || status.RobotScope != identity.RobotScope {
		return ErrMCPAccountChanged
	}
	if status.Enabled && !status.SuperAdmin {
		return ErrMCPServiceControl
	}
	if m.data.MCPAutoStart == enabled {
		return nil
	}
	m.data.MCPAutoStart = enabled
	return m.persist()
}
