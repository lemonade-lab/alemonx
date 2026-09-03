package redis

// The public Redis endpoint is intentionally a small TCP proxy.  Backends are
// never exposed on the configured port, which lets ALemonX prepare a private
// runtime on another loopback port without changing client configuration.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

type redisProxy struct {
	listener    net.Listener
	backend     atomic.Value // string
	closed      chan struct{}
	clients     sync.WaitGroup
	active      atomic.Int64
	connections sync.Map // net.Conn -> struct{}
}

func newRedisProxy(address, backend string) (*redisProxy, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	p := &redisProxy{listener: listener, closed: make(chan struct{})}
	p.backend.Store(backend)
	go p.serve()
	return p, nil
}

func (p *redisProxy) setBackend(address string) { p.backend.Store(address) }
func (p *redisProxy) activeClients() int64      { return p.active.Load() }

func (p *redisProxy) close() {
	if p == nil {
		return
	}
	_ = p.listener.Close()
	close(p.closed)
	p.connections.Range(func(key, _ any) bool { _ = key.(net.Conn).Close(); return true })
	p.clients.Wait()
}

func (p *redisProxy) serve() {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.closed:
				return
			default:
				continue
			}
		}
		p.clients.Add(1)
		p.connections.Store(client, struct{}{})
		go func() { defer p.clients.Done(); defer p.connections.Delete(client); p.handle(client) }()
	}
}

func (p *redisProxy) handle(client net.Conn) {
	defer client.Close()
	reader := bufio.NewReader(client)
	_, raw, err := readRESPCommand(reader)
	if err != nil {
		return
	}
	backend, err := net.Dial("tcp", p.backend.Load().(string))
	if err != nil {
		_, _ = client.Write([]byte("-LOADING Redis backend is preparing.\r\n"))
		return
	}
	defer backend.Close()
	backendReader := bufio.NewReader(backend)
	if _, err := backend.Write(raw); err != nil {
		return
	}
	p.active.Add(1)
	defer p.active.Add(-1)
	// Preserve all subsequent RESP bytes verbatim. This supports pipelines,
	// transactions, blocking commands and Pub/Sub without command rewriting.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(backend, reader); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, backendReader); done <- struct{}{} }()
	<-done
}

// readRESPCommand reads one array request while retaining the exact bytes so
// that a non-authenticated command can never be accidentally forwarded.
func readRESPCommand(reader *bufio.Reader) ([][]byte, []byte, error) {
	line, err := reader.ReadString('\n')
	if err != nil || len(line) < 4 || line[0] != '*' {
		return nil, nil, errors.New("invalid RESP command")
	}
	var count int
	if _, err := fmt.Sscanf(line, "*%d\r\n", &count); err != nil || count < 0 || count > 1024 {
		return nil, nil, errors.New("invalid RESP array")
	}
	raw := []byte(line)
	values := make([][]byte, 0, count)
	for range count {
		header, err := reader.ReadString('\n')
		if err != nil || len(header) < 4 || header[0] != '$' {
			return nil, nil, errors.New("invalid RESP bulk")
		}
		var size int
		if _, err := fmt.Sscanf(header, "$%d\r\n", &size); err != nil || size < 0 || size > 64<<20 {
			return nil, nil, errors.New("invalid RESP bulk size")
		}
		value := make([]byte, size+2)
		if _, err := io.ReadFull(reader, value); err != nil {
			return nil, nil, err
		}
		if value[size] != '\r' || value[size+1] != '\n' {
			return nil, nil, errors.New("invalid RESP terminator")
		}
		raw = append(raw, header...)
		raw = append(raw, value...)
		values = append(values, value[:size])
	}
	return values, raw, nil
}
