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
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const ErrBlockedHTTPStorageURI = "http(s) storageUri %q targets a blocked host or IP"

var lookupIPFn = net.LookupIP

// allowHTTPStorageLoopback is only for unit tests that use net/http/httptest.
// Production code must leave this disabled.
var allowHTTPStorageLoopback atomic.Bool

// AllowHTTPStorageLoopbackForTesting enables loopback http(s) storage URIs for tests.
func AllowHTTPStorageLoopbackForTesting(allow bool) {
	allowHTTPStorageLoopback.Store(allow)
}

var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

var blockedSpecialPurposePrefixes = []netip.Prefix{
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

// CheckHTTPStorageURI rejects http(s) URIs whose host is a blocked IP literal
// or a well-known internal/metadata hostname. Non-http(s) URIs are ignored.
// DNS is deliberately not resolved so admission remains offline-safe.
func CheckHTTPStorageURI(rawURI string) error {
	return checkHTTPStorageURI(rawURI, false)
}

// CheckHTTPStorageURIResolved also resolves hostnames. Fetchers should use this
// check and SafeHTTPClient to cover redirects and the final dial.
func CheckHTTPStorageURIResolved(rawURI string) error {
	return checkHTTPStorageURI(rawURI, true)
}

func checkHTTPStorageURI(rawURI string, resolve bool) error {
	parsed, err := url.Parse(rawURI)
	if err != nil {
		return fmt.Errorf("invalid storageUri: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf(ErrBlockedHTTPStorageURI, rawURI)
	}
	if err := checkHTTPStorageHost(host, resolve); err != nil {
		return fmt.Errorf(ErrBlockedHTTPStorageURI, rawURI)
	}
	return nil
}

func checkHTTPStorageHost(host string, resolve bool) error {
	if isBlockedHostname(host) || isBlockedIPLiteral(host) || isObfuscatedIPv4(host) {
		return fmt.Errorf("blocked host %q", host)
	}
	if !resolve {
		return nil
	}
	_, err := resolveHTTPStorageHost(host)
	return err
}

func resolveHTTPStorageHost(host string) ([]netip.Addr, error) {
	if isBlockedHostname(host) || isObfuscatedIPv4(host) {
		return nil, fmt.Errorf("blocked host %q", host)
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if isBlockedAddr(addr) {
			return nil, fmt.Errorf("blocked IP %s", addr)
		}
		return []netip.Addr{addr.Unmap()}, nil
	}
	ips, err := lookupIPFn(host)
	if err != nil {
		return nil, fmt.Errorf("resolve host %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("host %q resolved to no addresses", host)
	}
	addresses := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		if isBlockedAddr(addr) {
			return nil, fmt.Errorf("blocked resolved IP %s for host %q", addr, host)
		}
		addresses = append(addresses, addr)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("host %q resolved to no usable addresses", host)
	}
	return addresses, nil
}

func isBlockedHostname(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if allowHTTPStorageLoopback.Load() && (h == "localhost" || strings.HasSuffix(h, ".localhost")) {
		return false
	}
	switch h {
	case "localhost", "metadata", "metadata.google.internal",
		"kubernetes", "kubernetes.default", "kubernetes.default.svc",
		"kubernetes.default.svc.cluster.local",
		"host.docker.internal", "host.containers.internal":
		return true
	}
	return strings.HasSuffix(h, ".localhost") ||
		strings.HasSuffix(h, ".svc") ||
		strings.HasSuffix(h, ".cluster.local") ||
		strings.HasSuffix(h, ".internal")
}

func isBlockedIPLiteral(host string) bool {
	addr, err := netip.ParseAddr(host)
	return err == nil && isBlockedAddr(addr)
}

func isBlockedAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if allowHTTPStorageLoopback.Load() && addr.IsLoopback() {
		return false
	}
	if addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() || addr.IsUnspecified() || sharedAddressSpace.Contains(addr) {
		return true
	}
	for _, prefix := range blockedSpecialPurposePrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return addr.Is6() && !netip.MustParsePrefix("2000::/3").Contains(addr)
}

// Catch dword and octal spellings that URL parsers may pass to HTTP stacks.
func isObfuscatedIPv4(host string) bool {
	if _, err := strconv.ParseUint(host, 10, 32); err == nil {
		return true
	}
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if len(part) > 1 && part[0] == '0' {
			return true
		}
		if _, err := strconv.ParseUint(part, 10, 8); err != nil {
			return false
		}
	}
	return false
}

// SafeHTTPClient returns a shallow copy which checks redirect targets and
// rejects dials to internal addresses. Unknown RoundTrippers retain redirect
// protection but cannot be wrapped at the dial layer.
func SafeHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	out := *client
	originalRedirect := out.CheckRedirect
	out.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := CheckHTTPStorageURIResolved(req.URL.String()); err != nil {
			return err
		}
		if originalRedirect != nil {
			return originalRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	out.Transport = wrapSafeTransport(out.Transport)
	return &out
}

func wrapSafeTransport(roundTripper http.RoundTripper) http.RoundTripper {
	var transport *http.Transport
	switch typed := roundTripper.(type) {
	case *http.Transport:
		transport = typed.Clone()
	case nil:
		transport = http.DefaultTransport.(*http.Transport).Clone()
	default:
		return roundTripper
	}
	// A proxy resolves the destination outside our validated dial path.
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 30 * time.Second
	baseDial := transport.DialContext
	if baseDial == nil {
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		baseDial = dialer.DialContext
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid HTTP storage address %q: %w", address, err)
		}
		addresses, err := resolveHTTPStorageHost(host)
		if err != nil {
			return nil, fmt.Errorf(ErrBlockedHTTPStorageURI, address)
		}
		var lastErr error
		for _, resolved := range addresses {
			conn, dialErr := baseDial(ctx, network, net.JoinHostPort(resolved.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, fmt.Errorf("failed to dial validated addresses for %q: %w", host, lastErr)
	}
	return transport
}
