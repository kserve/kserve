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

package storage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

const maxHTTPSRedirects = 10

var blockedHTTPDestinationPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

type restrictedHTTPTransport struct {
	base http.RoundTripper
}

type httpHostResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type httpDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

func defaultHTTPStorageClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A proxy would resolve the target outside the validated dial path.
	transport.Proxy = nil
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport.DialContext = restrictedHTTPDialContext(dialer, net.DefaultResolver)

	return &http.Client{
		Transport:     restrictedHTTPTransport{base: transport},
		CheckRedirect: checkHTTPStorageRedirect,
	}
}

func (t restrictedHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := validateHTTPURL(req.Context(), req.URL); err != nil {
		return nil, err
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func restrictedHTTPDialContext(dialer httpDialer, resolver httpHostResolver) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addrs, err := resolveHTTPHost(ctx, resolver, host)
		if err != nil {
			return nil, err
		}

		var dialErrors []error
		for _, addr := range addrs {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(addr.Unmap().String(), port))
			if dialErr == nil {
				return conn, nil
			}
			dialErrors = append(dialErrors, dialErr)
		}
		return nil, fmt.Errorf("failed to dial HTTP(S) storage destination %q: %w", host, errors.Join(dialErrors...))
	}
}

func checkHTTPStorageRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxHTTPSRedirects {
		return errors.New("stopped after 10 redirects")
	}
	return validateHTTPURL(req.Context(), req.URL)
}

func validateHTTPURL(ctx context.Context, uri *url.URL) error {
	if uri == nil {
		return errors.New("HTTP(S) storage URI is empty")
	}
	if uri.Scheme != "http" && uri.Scheme != "https" {
		return fmt.Errorf("unsupported HTTP(S) storage URI scheme: %s", uri.Scheme)
	}
	if uri.Hostname() == "" {
		return fmt.Errorf("HTTP(S) storage URI %q does not include a host", uri.String())
	}
	return validateHTTPHost(ctx, uri.Hostname())
}

func validateHTTPHost(ctx context.Context, host string) error {
	_, err := resolveHTTPHost(ctx, net.DefaultResolver, host)
	return err
}

func resolveHTTPHost(ctx context.Context, resolver httpHostResolver, host string) ([]netip.Addr, error) {
	if host == "" {
		return nil, errors.New("HTTP(S) storage URI host is empty")
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		if err := validateHTTPDestinationAddr(host, addr); err != nil {
			return nil, err
		}
		return []netip.Addr{addr}, nil
	}

	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve HTTP(S) storage URI host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("HTTP(S) storage URI host %q resolved to no addresses", host)
	}
	for _, addr := range addrs {
		if err := validateHTTPDestinationAddr(host, addr); err != nil {
			return nil, err
		}
	}
	return addrs, nil
}

func validateHTTPDestinationAddr(host string, addr netip.Addr) error {
	addr = addr.Unmap().WithZone("")
	for _, prefix := range blockedHTTPDestinationPrefixes {
		if prefix.Contains(addr) {
			return fmt.Errorf("blocked unsafe HTTP(S) storage destination %q resolved to %s", host, addr)
		}
	}
	return nil
}
