package web

import (
	"context"
	"testing"
)

func TestServerRuntimeShutdownIsIdempotent(t *testing.T) {
	runtime := &ServerRuntime{server: &server{dshRuntimes: newDSHRegistry(), goalSchedulerStop: make(chan struct{})}}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
