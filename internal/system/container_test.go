package system

import "testing"

func TestInContainerDetection(t *testing.T) {
	t.Setenv("ALX_CONTAINER", "")
	t.Setenv("ALEMONJS_SETUP_ROOTS", "")
	if InContainer() {
		t.Fatal("host environment should not be detected as a container")
	}
	t.Setenv("ALX_CONTAINER", "1")
	if !InContainer() {
		t.Fatal("ALX_CONTAINER=1 should be detected as a container")
	}
	t.Setenv("ALX_CONTAINER", "")
	t.Setenv("ALEMONJS_SETUP_ROOTS", "/app/workspace")
	if !InContainer() {
		t.Fatal("ALEMONJS_SETUP_ROOTS under /app should be detected as a container")
	}
}
