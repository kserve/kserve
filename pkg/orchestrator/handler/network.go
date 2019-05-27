package handler

import (
	"crypto/tls"
	"golang.org/x/net/http2"
	"net"
	"net/http"
	"time"
)

const (
	// DefaultConnTimeout specifies a short default connection timeout
	// to avoid hitting the issue fixed in
	// https://github.com/kubernetes/kubernetes/pull/72534 but only
	// avalailable after Kubernetes 1.14.
	//
	// Our connections are usually between pods in the same cluster
	// like activator <-> queue-proxy, or even between containers
	// within the same pod queue-proxy <-> user-container, so a
	// smaller connect timeout would be justifiable.
	//
	// We should consider exposing this as a configuration.
	DefaultConnTimeout = 200 * time.Millisecond
)

// RoundTripperFunc implementation roundtrips a request.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (rt RoundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}

func newAutoTransport(v1 http.RoundTripper, v2 http.RoundTripper) http.RoundTripper {
	return RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		t := v1
		if r.ProtoMajor == 2 {
			t = v2
		}
		return t.RoundTrip(r)
	})
}

// NewH2CTransport constructs a new H2C transport.
// That transport will reroute all HTTPS traffic to HTTP. This is
// to explicitly allow h2c (http2 without TLS) transport.
// See https://github.com/golang/go/issues/14141 for more details.
func NewH2CTransport() http.RoundTripper {
	return &http2.Transport{
		AllowHTTP: true,
		DialTLS: func(netw, addr string, cfg *tls.Config) (net.Conn, error) {
			d := &net.Dialer{
				Timeout:   DefaultConnTimeout,
				KeepAlive: 5 * time.Second,
				DualStack: true,
			}
			return d.Dial(netw, addr)
		},
	}
}

// DefaultH2CTransport is a singleton instance of H2C transport.
var DefaultH2CTransport http.RoundTripper = NewH2CTransport()

func newHTTPTransport(connTimeout time.Duration) http.RoundTripper {
	return &http.Transport{
		// Those match net/http/transport.go
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       5 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		// This is bespoke.
		DialContext: (&net.Dialer{
			Timeout:   connTimeout,
			KeepAlive: 5 * time.Second,
			DualStack: true,
		}).DialContext,
	}
}

// NewAutoTransport creates a RoundTripper that can use appropriate transport
// based on the request's HTTP version.
func NewAutoTransport() http.RoundTripper {
	return newAutoTransport(newHTTPTransport(DefaultConnTimeout), NewH2CTransport())
}

// AutoTransport uses h2c for HTTP2 requests and falls back to `http.DefaultTransport` for all others
var AutoTransport = NewAutoTransport()
