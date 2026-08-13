package system

import "testing"

func TestValidatedServiceLogLines(t *testing.T) {
	if got, err := validatedServiceLogLines(0); err != nil || got != defaultServiceLogLines {
		t.Fatalf("default lines = %d, %v", got, err)
	}
	if got, err := validatedServiceLogLines(42); err != nil || got != 42 {
		t.Fatalf("explicit lines = %d, %v", got, err)
	}
	if _, err := validatedServiceLogLines(maxServiceLogLines + 1); err == nil {
		t.Fatal("expected excessive line count to fail")
	}
}
