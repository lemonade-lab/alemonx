package systemnetwork

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Dialer applies the same global policy to application-owned TCP clients.
// Database drivers keep their own TLS negotiation above this connection.
type Dialer struct{ Timeout time.Duration }

func (d Dialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}
func (d Dialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return (Dialer{Timeout: timeout}).Dial(network, address)
}
func (d Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if d.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.Timeout)
		defer cancel()
	}
	defaultMu.RLock()
	m := defaultManager
	defaultMu.RUnlock()
	c := m.ConfigSnapshot()
	if c != nil && c.Mode == "proxy" && !bypassProxy(&url.URL{Host: address}) && network != "unix" {
		if network != "tcp" && network != "tcp4" && network != "tcp6" {
			return nil, errors.New("当前代理不支持此网络协议")
		}
		conn, err := dialProxy(ctx, c.Proxy.URL, address)
		if err != nil {
			return nil, errors.New("全局代理连接失败")
		}
		return conn, nil
	}
	return (&net.Dialer{Timeout: d.Timeout}).DialContext(ctx, network, address)
}

// LocalClient must never inherit external proxies, even before migration.
func LocalClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: directTransport(), CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if !bypassProxy(r.URL) || len(via) >= 10 {
			return errors.New("本地服务不允许外部重定向")
		}
		return nil
	}}
}
