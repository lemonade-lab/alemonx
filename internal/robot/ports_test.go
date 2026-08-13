package robot

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPortsReturnsAppAndTestPorts(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeWebViewFixture(t, filepath.Join(root, "alemon.config.yaml"), "serverPort: 19191\nport: 17222\n")
	ports, err := (Manager{}).Ports(root)
	if err != nil {
		t.Fatalf("Ports: %v", err)
	}
	want := []RobotPort{
		{Kind: "app", Label: "应用端口", Port: 19191, Configured: true},
		{Kind: "test", Label: "测试端口", Port: 17222, Configured: true},
	}
	if !reflect.DeepEqual(ports, want) {
		t.Fatalf("Ports = %#v, want %#v", ports, want)
	}
}

func TestPortsReturnsNothingWithoutConfig(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	ports, err := (Manager{}).Ports(root)
	if err != nil {
		t.Fatalf("Ports: %v", err)
	}
	if len(ports) != 0 {
		t.Fatalf("Ports = %#v, want none when alemon.config.yaml declares no ports", ports)
	}
}

func TestPortsReturnsOnlyConfiguredPorts(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeWebViewFixture(t, filepath.Join(root, "alemon.config.yaml"), "serverPort: 19191\n")
	ports, err := (Manager{}).Ports(root)
	if err != nil {
		t.Fatalf("Ports: %v", err)
	}
	want := []RobotPort{
		{Kind: "app", Label: "应用端口", Port: 19191, Configured: true},
	}
	if !reflect.DeepEqual(ports, want) {
		t.Fatalf("Ports = %#v, want only the configured app port %#v", ports, want)
	}
}
