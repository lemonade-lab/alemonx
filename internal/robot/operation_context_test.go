package robot

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestOperationCancellationStopsCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process tree fixture")
	}
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	release, err := BeginOperation(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := BeginOperation(ctx, root); err == nil {
		t.Fatal("overlapping task accepted")
	}
	done := make(chan error, 1)
	go func() { _, err := run(root, "sh", "-c", "sleep 60 & wait"); done <- err }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled command succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not terminate command tree")
	}
	release()
	second, err := BeginOperation(context.Background(), root)
	if err != nil {
		t.Fatal("directory still locked:", err)
	}
	defer second()
	// Releasing an old handle twice must not remove a new worker's ownership.
	release()
	if _, err := BeginOperation(context.Background(), root); err == nil {
		t.Fatal("old cleanup removed new owner")
	}
}
