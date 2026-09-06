package system

import (
	"context"
	"strings"
	"testing"
)

func TestContainerDisablesRuntimeAndServiceManagement(t *testing.T) {
	t.Setenv("ALX_CONTAINER", "1")

	python := PythonRuntimeStatusForHost()
	if !python.Fixed || python.Versions == nil {
		t.Fatalf("PythonRuntimeStatusForHost() = %#v, want fixed status and a non-nil version list", python)
	}
	for _, operation := range []func() (string, error){
		func() (string, error) { return InstallEnvironment(context.Background(), "git") },
		func() (string, error) { return InstallPythonVersion(context.Background(), "3.12.10") },
		func() (string, error) { return UsePythonVersion(context.Background(), "3.12.10") },
		func() (string, error) { return SetStartupEnabled(true) },
		func() (string, error) { return EnableUserLinger() },
		func() (string, error) { return StartService() },
		func() (string, error) { return StopService() },
		func() (string, error) { return PrepareService("1717") },
		func() (string, error) { return UninstallService() },
	} {
		if _, err := operation(); err == nil || !strings.Contains(err.Error(), "Docker") {
			t.Fatalf("container operation error = %v, want Docker guidance", err)
		}
	}
	if err := ScheduleServiceStart(); err == nil || !strings.Contains(err.Error(), "Docker") {
		t.Fatalf("ScheduleServiceStart() error = %v, want Docker guidance", err)
	}
	if err := RestartForeground("1717"); err == nil || !strings.Contains(err.Error(), "Docker") {
		t.Fatalf("RestartForeground() error = %v, want Docker guidance", err)
	}
}
