package system

import "testing"

func TestConfigurePrivilegedModeRejectsNonLoopbackLocal(t *testing.T) {
	t.Setenv("ALX_PRIVILEGED_MODE", "local")
	if err := ConfigurePrivilegedMode("0.0.0.0", false); err == nil {
		t.Fatal("non-loopback local privilege mode must be rejected")
	}
}

func TestConfigurePrivilegedModeDefaultsToEnabledBroker(t *testing.T) {
	t.Setenv("ALX_PRIVILEGED_MODE", "")
	if err := ConfigurePrivilegedMode("0.0.0.0", true); err != nil {
		t.Fatal(err)
	}
	status := CurrentPrivilegeStatus()
	if !status.Enabled || status.Mode != string(PrivilegedModeEnabled) || status.Version != "broker-v2" {
		t.Fatalf("privilege status = %#v", status)
	}
	t.Cleanup(func() { t.Setenv("ALX_PRIVILEGED_MODE", "disabled"); _ = ConfigurePrivilegedMode("127.0.0.1", false) })
}
