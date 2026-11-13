package web

func (s *server) closeTerminalSessions() {
	if s.terminalSessions == nil {
		return
	}
	s.terminalSessions.mu.Lock()
	ids := make([]string, 0, len(s.terminalSessions.sessions))
	for id := range s.terminalSessions.sessions {
		ids = append(ids, id)
	}
	s.terminalSessions.mu.Unlock()
	for _, id := range ids {
		s.closeTerminalSession(id)
	}
}
