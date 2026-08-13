package system

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestSudoPasswordReaderClearsTemporaryBuffer(t *testing.T) {
	reader := newSudoPasswordReader([]byte("correct"))
	output, err := io.ReadAll(reader)
	if err != nil || string(output) != "correct\n" {
		t.Fatalf("password input = %q, err=%v", output, err)
	}
	for _, value := range reader.secret {
		if value != 0 {
			t.Fatal("temporary password buffer was not cleared")
		}
	}
}

func TestRunSudoCommandClearsPasswordAndUsesDeclaredOperation(t *testing.T) {
	previous := sudoCommand
	t.Cleanup(func() { sudoCommand = previous })
	called := false
	sudoCommand = func(_ context.Context, password []byte, program string, args []string) ([]byte, error) {
		called = true
		if string(password) != "correct" {
			t.Fatalf("password = %q", password)
		}
		if program != "go" || len(args) != 1 || args[0] != "version" {
			t.Fatalf("declared command = %s %#v", program, args)
		}
		return []byte("installed"), nil
	}
	password := []byte("correct")
	if _, err := RunSudoCommand(context.Background(), password, "go", []string{"version"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("fixed sudo operation was not called")
	}
	for _, value := range password {
		if value != 0 {
			t.Fatal("password bytes were not cleared")
		}
	}
}

func TestRunSudoCommandClassifiesBadPassword(t *testing.T) {
	previous := sudoCommand
	t.Cleanup(func() { sudoCommand = previous })
	sudoCommand = func(context.Context, []byte, string, []string) ([]byte, error) {
		return []byte("Sorry, try again."), errors.New("exit status 1")
	}
	_, err := RunSudoCommand(context.Background(), []byte("wrong"), "go", []string{"version"})
	if err == nil || err.Error() != "sudo 密码无效，请确认后重试" {
		t.Fatalf("bad password error = %v", err)
	}
}
