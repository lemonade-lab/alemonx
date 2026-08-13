package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivilegedHelperRunsDirectProgram(t *testing.T) {
	if testing.Short() || os.PathSeparator == '\\' {
		t.Skip("uses a small Unix shell fixture")
	}
	directory := t.TempDir()
	program := filepath.Join(directory, "runner")
	if err := os.WriteFile(program, []byte("#!/bin/sh\ncat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(directory, "input.json")
	output := filepath.Join(directory, "output.json")
	errorPath := filepath.Join(directory, "error.txt")
	if err := os.WriteFile(input, []byte(`{"action":"apply"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := RunPrivilegedHelper([]string{"--directory", directory, "--input", input, "--output", output, "--error", errorPath, "--program", program}); code != 0 {
		t.Fatalf("helper exit = %d", code)
	}
	value, err := os.ReadFile(output)
	if err != nil || string(value) != `{"action":"apply"}` {
		t.Fatalf("helper output = %q, %v", value, err)
	}
}
