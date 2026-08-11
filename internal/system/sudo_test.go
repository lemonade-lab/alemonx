package system

import (
	"context"
	"errors"
	"io"
	"reflect"
	"runtime"
	"testing"
)

func TestNapcatAPTCommandIsFixed(t *testing.T) {
	command := napcatAPTCommand(context.Background(), []byte("not-used"))
	want := []string{"sudo", "-S", "-k", "-p", "", "--", "apt-get", "install", "-y", "xvfb", "libnss3", "libgbm1", "libglib2.0-0", "libatk1.0-0", "libatspi2.0-0", "libgtk-3-0", "libasound2"}
	if !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("sudo command = %#v, want %#v", command.Args, want)
	}
}

func TestNapcatDNFCommandIsFixed(t *testing.T) {
	command := napcatDNFCommand(context.Background(), []byte("not-used"))
	want := []string{"sudo", "-S", "-k", "-p", "", "--", "dnf", "install", "--allowerasing", "-y", "xorg-x11-server-Xvfb", "nss", "mesa-libgbm", "glib2", "atk", "at-spi2-atk", "gtk3", "alsa-lib"}
	if !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("sudo command = %#v, want %#v", command.Args, want)
	}
}

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

func TestInstallNapCatAPTDependenciesClearsPasswordAndUsesFixedOperation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("APT operation is Linux-only")
	}
	previous := sudoCommand
	t.Cleanup(func() { sudoCommand = previous })
	called := false
	sudoCommand = func(_ context.Context, password []byte) ([]byte, error) {
		called = true
		if string(password) != "correct" {
			t.Fatalf("password = %q", password)
		}
		return []byte("installed"), nil
	}
	password := []byte("correct")
	if _, err := InstallNapCatAPTDependencies(context.Background(), password); err != nil {
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

func TestInstallNapCatAPTDependenciesClassifiesBadPassword(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("APT operation is Linux-only")
	}
	previous := sudoCommand
	t.Cleanup(func() { sudoCommand = previous })
	sudoCommand = func(context.Context, []byte) ([]byte, error) {
		return []byte("Sorry, try again."), errors.New("exit status 1")
	}
	_, err := InstallNapCatAPTDependencies(context.Background(), []byte("wrong"))
	if err == nil || err.Error() != "sudo 密码无效，请确认后重试" {
		t.Fatalf("bad password error = %v", err)
	}
}
