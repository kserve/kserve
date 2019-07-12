/*
Copyright 2019 The Knative Authors

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

// spoof contains logic to make polling HTTP requests against an endpoint with optional host spoofing.

package spoof

import (
	"net/http"
	"testing"
)

func TestDNSError(t *testing.T) {
	client := &http.Client{}

	for _, tt := range []struct {
		name     string
		url      string
		dnsError bool
	}{{
		name:     "url does not exist",
		url:      "http://this.url.does.not.exist",
		dnsError: true,
	}, {
		name:     "ip address",
		url:      "http://127.0.0.1",
		dnsError: false,
	}, {
		name:     "localhost",
		url:      "http://localhost:8080",
		dnsError: false,
	}, {
		name:     "no error",
		url:      "http://google.com",
		dnsError: false,
	}} {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			_, err := client.Do(req)
			if dnsError := isDNSError(err); tt.dnsError != dnsError {
				t.Errorf("Expected dnsError=%v, got %v", tt.dnsError, dnsError)
			}
		})
	}
}

func TestTCPConnectRefuse(t *testing.T) {
	client := &http.Client{}

	for _, tt := range []struct {
		name      string
		url       string
		tcpRefuse bool
	}{{
		name:      "nothing listening",
		url:       "http://localhost:60001",
		tcpRefuse: true,
	}, {
		name:      "dns error",
		url:       "http://this.url.does.not.exist",
		tcpRefuse: false,
	}, {
		name:      "google.com",
		url:       "https://google.com",
		tcpRefuse: false,
	}} {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			_, err := client.Do(req)
			if tcpRefuse := isTCPConnectRefuse(err); tt.tcpRefuse != tcpRefuse {
				t.Errorf("Expected tcpRefuse=%v, got %v", tt.tcpRefuse, tcpRefuse)
			}
		})
	}
}

func TestTCPTimeout(t *testing.T) {
	client := &http.Client{}

	// We have no positive test for TCP timeout, but we do have a few negative tests.
	for _, tt := range []struct {
		name       string
		url        string
		tcpTimeout bool
	}{{
		name:       "nothing listening",
		url:        "http://localhost:60001",
		tcpTimeout: false,
	}, {
		name:       "dns error",
		url:        "http://this.url.does.not.exist",
		tcpTimeout: false,
	}, {
		name:       "google.com",
		url:        "https://google.com",
		tcpTimeout: false,
	}} {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			_, err := client.Do(req)
			if tcpTimeout := isTCPTimeout(err); tt.tcpTimeout != tcpTimeout {
				t.Errorf("Expected tcpTimeout=%v, got %v", tt.tcpTimeout, tcpTimeout)
			}
		})
	}
}
