package setup

import (
	"net"
	"net/http"
	"time"
)

// NewHTTPClient returns a pooled client with keep-alive and a hard timeout.
func NewHTTPClient(maxSockets int, timeout time.Duration) *http.Client {
	if maxSockets <= 0 {
		maxSockets = 10
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          maxSockets * 2,
		MaxIdleConnsPerHost:   maxSockets,
		MaxConnsPerHost:       maxSockets,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

// HTTPClients holds one pooled client per upstream.
type HTTPClients struct {
	Recommendation, OrderService, TROrderService, Shopflo, CMS, TRCMS, ConfigService *http.Client
}

// NewHTTPClients builds clients sized from cfg.MaxSockets.
func NewHTTPClients(cfg *Config) *HTTPClients {
	mk := func(name string) *http.Client { return NewHTTPClient(cfg.MaxSockets[name], cfg.HTTPTimeout) }
	return &HTTPClients{
		Recommendation: mk("recommendation"), OrderService: mk("order"), TROrderService: mk("trorder"),
		Shopflo: mk("shopflo"), CMS: mk("cms"), TRCMS: mk("trcms"), ConfigService: mk("config"),
	}
}
