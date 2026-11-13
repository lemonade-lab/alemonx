package main

import (
	"net"
	"strconv"
	"testing"
)

func TestListenWorkbenchPortConflict(t *testing.T) {
	const host = "127.0.0.1"
	occupied, err := net.Listen("tcp", host+":0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := strconv.Itoa(occupied.Addr().(*net.TCPAddr).Port)

	if listener, err := listenWorkbench(host, port, false); err == nil {
		listener.Close()
		t.Fatal("web server accepted an occupied port")
	}
	listener, err := listenWorkbench(host, port, true)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	addr := listener.Addr().(*net.TCPAddr)
	if addr.IP.String() != host || strconv.Itoa(addr.Port) == port {
		t.Fatalf("expected a different loopback port, got %s", addr)
	}
}
