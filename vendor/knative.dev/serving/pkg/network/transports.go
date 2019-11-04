/*
Copyright 2018 The Knative Authors
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package network

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
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

const (
	initialTO = float64(50 * time.Millisecond)
	sleepTO   = 30 * time.Millisecond
	factor    = 1.4
	numSteps  = 15
)

var errDialTimeout = errors.New("timed out dialing")

// dialWithBackOff executes `net.Dialer.DialContext()` with exponentially increasing
// dial timeouts. In addition it sleeps with random jitter between tries.
func dialWithBackOff(ctx context.Context, network, address string) (net.Conn, error) {
	return dialBackOffHelper(ctx, network, address, numSteps, initialTO, sleepTO)
}

func dialBackOffHelper(ctx context.Context, network, address string, steps int, initial float64, sleep time.Duration) (net.Conn, error) {
	to := initial
	dialer := &net.Dialer{
		Timeout:   time.Duration(to),
		KeepAlive: 5 * time.Second,
		DualStack: true,
	}
	// TODO(vagababov): use backoff.Step when we use modern k8s client.
	for i := 0; i < steps; i++ {
		c, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				if i == steps-1 {
					break
				}
				to *= factor
				dialer.Timeout = time.Duration(to)
				time.Sleep(wait.Jitter(sleep, 1.0)) // Sleep with jitter.
				continue
			}
			return nil, err
		}
		return c, err
	}
	return nil, errDialTimeout
}

func newHTTPTransport(connTimeout time.Duration, disableKeepAlives bool) http.RoundTripper {
	return &http.Transport{
		// Those match net/http/transport.go
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       5 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     disableKeepAlives,

		// This is bespoke.
		DialContext: dialWithBackOff,
	}
}

// NewProberTransport creates a RoundTripper that is useful for probing,
// since it will not cache connections.
func NewProberTransport() http.RoundTripper {
	return newAutoTransport(
		newHTTPTransport(DefaultConnTimeout, true /*disable keep-alives*/),
		NewH2CTransport())
}

// NewAutoTransport creates a RoundTripper that can use appropriate transport
// based on the request's HTTP version.
func NewAutoTransport() http.RoundTripper {
	return newAutoTransport(
		newHTTPTransport(DefaultConnTimeout, false /*disable keep-alives*/),
		NewH2CTransport())
}

// AutoTransport uses h2c for HTTP2 requests and falls back to `http.DefaultTransport` for all others
var AutoTransport = NewAutoTransport()
