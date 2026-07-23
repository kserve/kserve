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
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

type restrictedHTTPTransport struct {
	base http.RoundTripper
}

func defaultHTTPStorageClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport.DialContext = restrictedHTTPDialContext(dialer)

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

func restrictedHTTPDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if err := validateHTTPHost(ctx, host); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
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
	if host == "" {
		return errors.New("HTTP(S) storage URI host is empty")
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		return validateHTTPDestinationAddr(host, addr)
	}

	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("failed to resolve HTTP(S) storage URI host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("HTTP(S) storage URI host %q resolved to no addresses", host)
	}
	for _, addr := range addrs {
		if err := validateHTTPDestinationAddr(host, addr); err != nil {
			return err
		}
	}
	return nil
}

func validateHTTPDestinationAddr(host string, addr netip.Addr) error {
	addr = addr.Unmap()
	for _, prefix := range blockedHTTPDestinationPrefixes {
		if prefix.Contains(addr) {
			return fmt.Errorf("blocked unsafe HTTP(S) storage destination %q resolved to %s", host, addr)
		}
	}
	return nil
}
