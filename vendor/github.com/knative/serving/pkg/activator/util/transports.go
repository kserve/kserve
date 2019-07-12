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

package util

import (
	"net"
	"net/http"
	"time"

	h2cutil "github.com/knative/serving/pkg/http/h2c"
	"github.com/knative/serving/pkg/network"
)

// RoundTripperFunc implementation roundtrips a request.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (rt RoundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}

// NewAutoTransport will use the appropriate transport for the request's HTTP protocol version
func NewAutoTransport(v1 http.RoundTripper, v2 http.RoundTripper) http.RoundTripper {
	return RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		t := v1
		if r.ProtoMajor == 2 {
			t = v2
		}

		return t.RoundTrip(r)
	})
}

func newHTTPTransport(connTimeout time.Duration) http.RoundTripper {
	transport := *http.DefaultTransport.(*http.Transport)
	transport.DialContext = (&net.Dialer{
		Timeout:   connTimeout,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}).DialContext
	return &transport
}

// AutoTransport uses h2c for HTTP2 requests and falls back to `http.DefaultTransport` for all others
var AutoTransport = NewAutoTransport(newHTTPTransport(network.DefaultConnTimeout), h2cutil.DefaultTransport)
