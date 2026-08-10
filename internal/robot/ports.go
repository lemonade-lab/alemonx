package robot

// RobotPort describes one port the robot binds while it runs. The app port
// (serverPort in alemon.config.yaml) serves the Koa application and plugin
// APIs; the test port (top-level port) serves the CBP/sandbox test platform.
type RobotPort struct {
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	Port       int    `json:"port"`
	Configured bool   `json:"configured"`
}

// Ports returns every port the robot expects to listen on when it starts.
// Both ports share the same defaults when alemon.config.yaml declares nothing,
// so the list may contain duplicates; callers can deduplicate by port.
func (m Manager) Ports(root string) ([]RobotPort, error) {
	app, err := m.AppPort(root)
	if err != nil {
		return nil, err
	}
	test, err := m.TestPort(root)
	if err != nil {
		return nil, err
	}
	return []RobotPort{
		{Kind: "app", Label: "应用端口", Port: app.Port, Configured: app.Configured},
		{Kind: "test", Label: "测试端口", Port: test.Port, Configured: test.Configured},
	}, nil
}
