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

// Ports returns the ports explicitly declared in alemon.config.yaml that the
// robot will bind when it starts. Ports that would fall back to framework
// defaults are deliberately excluded: when a port is not configured the
// workbench must not sniff it or show its status.
func (m Manager) Ports(root string) ([]RobotPort, error) {
	app, err := m.AppPort(root)
	if err != nil {
		return nil, err
	}
	test, err := m.TestPort(root)
	if err != nil {
		return nil, err
	}
	items := make([]RobotPort, 0, 2)
	if app.Configured {
		items = append(items, RobotPort{Kind: "app", Label: "应用端口", Port: app.Port, Configured: true})
	}
	if test.Configured {
		items = append(items, RobotPort{Kind: "test", Label: "服务端口", Port: test.Port, Configured: true})
	}
	return items, nil
}
