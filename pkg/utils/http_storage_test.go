/*
Copyright 2026 The KServe Authors.

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

package utils

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestCheckHTTPStorageURI(t *testing.T) {
	tests := []struct {
		uri     string
		blocked bool
	}{
		{uri: "s3://bucket/model"},
		{uri: "https://example.com/model.joblib"},
		{uri: "https://kfserving.blob.core.windows.net/triton/model"},
		{uri: "http://127.0.0.1/model", blocked: true},
		{uri: "http://[::1]/model", blocked: true},
		{uri: "http://localhost/model", blocked: true},
		{uri: "http://169.254.169.254/latest/meta-data", blocked: true},
		{uri: "http://10.0.0.1/model", blocked: true},
		{uri: "http://100.64.0.1/model", blocked: true},
		{uri: "http://100.127.255.254/model", blocked: true},
		{uri: "https://kubernetes.default.svc/api", blocked: true},
		{uri: "http://2130706433/model", blocked: true},
		{uri: "http://0177.0.0.1/model", blocked: true},
		{uri: "http://[fd00::1]/model", blocked: true},
		{uri: "http://192.0.2.1/model", blocked: true},
		{uri: "http://198.18.0.1/model", blocked: true},
		{uri: "http://203.0.113.1/model", blocked: true},
		{uri: "http://[2001:db8::1]/model", blocked: true},
		{uri: "http://[2002::1]/model", blocked: true},
	}
	for _, test := range tests {
		t.Run(test.uri, func(t *testing.T) {
			err := CheckHTTPStorageURI(test.uri)
			if (err != nil) != test.blocked {
				t.Fatalf("CheckHTTPStorageURI() error = %v, blocked = %v", err, test.blocked)
			}
		})
	}
}

func TestCheckHTTPStorageURIWithAllowlist(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		allowlist []string
		wantErr   bool
	}{
		{name: "private IP requires CIDR", uri: "http://10.20.1.4/model", wantErr: true},
		{name: "private IP allowed by CIDR", uri: "http://10.20.1.4/model", allowlist: []string{"10.20.0.0/16"}},
		{name: "private host allowed exactly", uri: "http://models.ml-platform.svc/model", allowlist: []string{"models.ml-platform.svc"}},
		{name: "private host is not wildcard matched", uri: "http://other.ml-platform.svc/model", allowlist: []string{"models.ml-platform.svc"}, wantErr: true},
		{name: "metadata stays blocked", uri: "http://169.254.169.254/latest/meta-data", allowlist: []string{"169.254.0.0/16"}, wantErr: true},
		{name: "IPv6 metadata stays blocked", uri: "http://[fd00:ec2::254]/latest/meta-data", allowlist: []string{"fd00::/8"}, wantErr: true},
		{name: "metadata CIDR is rejected", uri: "http://example.com/model", allowlist: []string{"169.254.0.0/16"}, wantErr: true},
		{name: "invalid wildcard is rejected", uri: "http://example.com/model", allowlist: []string{"*.ml-platform.svc"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := CheckHTTPStorageURIWithAllowlist(test.uri, test.allowlist)
			if (err != nil) != test.wantErr {
				t.Fatalf("CheckHTTPStorageURIWithAllowlist() error = %v, wantErr = %v", err, test.wantErr)
			}
		})
	}
}

func TestCheckHTTPStorageURIResolvedWithAllowlist(t *testing.T) {
	original := lookupIPFn
	t.Cleanup(func() { lookupIPFn = original })
	lookupIPFn = func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.20.1.4")}, nil
	}

	if err := CheckHTTPStorageURIResolvedWithAllowlist("http://models.example/model", []string{"10.20.0.0/16"}); err != nil {
		t.Fatalf("allowlisted private resolution failed: %v", err)
	}
	if err := CheckHTTPStorageURIResolvedWithAllowlist("http://models.example/model", []string{"10.30.0.0/16"}); err == nil {
		t.Fatal("expected private address outside the allowlist to be blocked")
	}
	if err := CheckHTTPStorageURIResolvedWithAllowlist("http://models.example/model", []string{"models.example"}); err != nil {
		t.Fatalf("exact allowlisted host failed: %v", err)
	}
}

func TestSafeHTTPClientDisablesProxy(t *testing.T) {
	client := SafeHTTPClient(&http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}})
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("expected proxy routing to be disabled")
	}
	if transport.ResponseHeaderTimeout != 30*time.Second {
		t.Fatalf("response header timeout = %s, want 30s", transport.ResponseHeaderTimeout)
	}
}

func TestSafeHTTPClientBlocksDNSRebinding(t *testing.T) {
	original := lookupIPFn
	t.Cleanup(func() { lookupIPFn = original })
	lookups := 0
	lookupIPFn = func(string) ([]net.IP, error) {
		lookups++
		if lookups == 1 {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return []net.IP{net.ParseIP("10.0.0.1")}, nil
	}

	if err := CheckHTTPStorageURIResolved("https://rebind.example/model"); err != nil {
		t.Fatalf("initial public resolution failed: %v", err)
	}

	baseDialCalled := false
	baseTransport := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			baseDialCalled = true
			return nil, errors.New("unexpected dial")
		},
	}
	client := SafeHTTPClient(&http.Client{Transport: baseTransport})
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(t.Context(), "tcp", "rebind.example:443"); err == nil {
		t.Fatal("expected rebound private address to be rejected")
	}
	if baseDialCalled {
		t.Fatal("private rebound address reached the underlying dialer")
	}
}

func TestSafeHTTPClientDialsValidatedIP(t *testing.T) {
	original := lookupIPFn
	t.Cleanup(func() { lookupIPFn = original })
	lookupIPFn = func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}

	var dialedAddress string
	baseTransport := &http.Transport{
		DialContext: func(_ context.Context, _, address string) (net.Conn, error) {
			dialedAddress = address
			return nil, errors.New("stop after capturing address")
		},
	}
	client := SafeHTTPClient(&http.Client{Transport: baseTransport})
	transport := client.Transport.(*http.Transport)
	_, _ = transport.DialContext(t.Context(), "tcp", "public.example:443")
	if dialedAddress != "93.184.216.34:443" {
		t.Fatalf("dialed %q, want validated IP", dialedAddress)
	}
}

func TestCheckHTTPStorageURIResolvedPrivateDNS(t *testing.T) {
	original := lookupIPFn
	t.Cleanup(func() { lookupIPFn = original })
	lookupIPFn = func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("169.254.169.254")}, nil
	}
	if err := CheckHTTPStorageURIResolved("http://evil.example/model"); err == nil {
		t.Fatal("expected resolved metadata address to be blocked")
	}
}

func TestSafeHTTPClientRejectsLoopbackRedirectAndDial(t *testing.T) {
	client := SafeHTTPClient(nil)
	request, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/model", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(request, nil); err == nil {
		t.Fatal("expected redirect to loopback to be blocked")
	}
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(t.Context(), "tcp", "127.0.0.1:1"); err == nil {
		t.Fatal("expected loopback dial to be blocked")
	}
}
