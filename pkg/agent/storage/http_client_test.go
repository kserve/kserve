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
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type staticHTTPResolver struct {
	addrs []netip.Addr
}

func (r staticHTTPResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.addrs, nil
}

func TestValidateHTTPDialAddressRejectsUnsafeResolvedAddress(t *testing.T) {
	err := validateHTTPDialAddress(context.Background(), "tcp4", "127.0.0.1:80", nil)
	if err == nil || !strings.Contains(err.Error(), "blocked unsafe HTTP(S) storage destination") {
		t.Fatalf("expected unsafe destination error, got: %v", err)
	}

	if err := validateHTTPDialAddress(context.Background(), "tcp4", "93.184.216.34:443", nil); err != nil {
		t.Fatalf("expected public dial address to be allowed: %v", err)
	}
}

func TestResolveHTTPHostRejectsMixedPublicAndPrivateAddresses(t *testing.T) {
	resolver := staticHTTPResolver{addrs: []netip.Addr{
		netip.MustParseAddr("93.184.216.34"),
		netip.MustParseAddr("10.0.0.1"),
	}}

	_, err := resolveHTTPHost(context.Background(), resolver, "models.example")
	if err == nil || !strings.Contains(err.Error(), "blocked unsafe HTTP(S) storage destination") {
		t.Fatalf("expected mixed DNS response to be rejected, got: %v", err)
	}
}

func TestValidateHTTPDestinationAddrRejectsIPv6ZoneBypass(t *testing.T) {
	err := validateHTTPDestinationAddr("fe80::1%lo0", netip.MustParseAddr("fe80::1%lo0"))
	if err == nil || !strings.Contains(err.Error(), "blocked unsafe HTTP(S) storage destination") {
		t.Fatalf("expected zoned link-local address to be rejected, got: %v", err)
	}
}

func TestDefaultHTTPStorageClientDoesNotUseEnvironmentProxy(t *testing.T) {
	client := defaultHTTPStorageClient()
	restrictedTransport, ok := client.Transport.(restrictedHTTPTransport)
	if !ok {
		t.Fatalf("transport type = %T, want restrictedHTTPTransport", client.Transport)
	}
	transport, ok := restrictedTransport.base.(*http.Transport)
	if !ok {
		t.Fatalf("base transport type = %T, want *http.Transport", restrictedTransport.base)
	}
	if transport.Proxy != nil {
		t.Fatal("HTTP storage client must not delegate target resolution to a proxy")
	}
	if transport.DialContext == nil {
		t.Fatal("HTTP storage client must validate resolved socket addresses")
	}
	if transport.ResponseHeaderTimeout != httpStorageResponseHeaderTimeout {
		t.Fatalf("response header timeout = %s, want %s", transport.ResponseHeaderTimeout, httpStorageResponseHeaderTimeout)
	}
}

func TestValidateHTTPDestinationAddrRejectsSpecialRanges(t *testing.T) {
	blocked := []string{
		"192.88.99.2",
		"100:0:0:1::1",
		"3fff::1",
		"5f00::1",
		"fec0::1",
		"4000::1",
	}
	for _, address := range blocked {
		t.Run(address, func(t *testing.T) {
			if err := validateHTTPDestinationAddr(address, netip.MustParseAddr(address)); err == nil {
				t.Fatalf("expected special address %s to be rejected", address)
			}
		})
	}

	allowed := []string{"93.184.216.34", "2606:4700:4700::1111"}
	for _, address := range allowed {
		t.Run(address, func(t *testing.T) {
			if err := validateHTTPDestinationAddr(address, netip.MustParseAddr(address)); err != nil {
				t.Fatalf("expected public address %s to be allowed: %v", address, err)
			}
		})
	}
}

type httpResolverFunc func(context.Context, string, string) ([]netip.Addr, error)

func (f httpResolverFunc) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return f(ctx, network, host)
}

func TestResolveHTTPHostNormalizesInternationalizedHostnames(t *testing.T) {
	for _, tt := range []struct {
		host string
		want string
	}{
		{host: "bücher.example", want: "xn--bcher-kva.example"},
		{host: "faß.example", want: "xn--fa-hia.example"},
		{host: "BÜCHER.Example", want: "xn--bcher-kva.example"},
		{host: "Models_Store.Example", want: "Models_Store.Example"},
	} {
		t.Run(tt.host, func(t *testing.T) {
			publicAddr := netip.MustParseAddr("93.184.216.34")
			resolver := httpResolverFunc(func(_ context.Context, network, host string) ([]netip.Addr, error) {
				if network != "ip" || host != tt.want {
					t.Fatalf("lookup = (%q, %q), want (%q, %q)", network, host, "ip", tt.want)
				}
				return []netip.Addr{publicAddr}, nil
			})
			addrs, err := resolveHTTPHost(context.Background(), resolver, tt.host)
			if err != nil {
				t.Fatalf("expected hostname to resolve: %v", err)
			}
			if len(addrs) != 1 || addrs[0] != publicAddr {
				t.Fatalf("resolved addresses = %v, want [%s]", addrs, publicAddr)
			}
		})
	}
}

func TestResolveHTTPHostBoundsDNSLookup(t *testing.T) {
	for _, tt := range []struct {
		name          string
		callerTimeout time.Duration
	}{
		{name: "no caller deadline"},
		{name: "earlier caller deadline", callerTimeout: 10 * time.Second},
		{name: "later caller deadline", callerTimeout: time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.callerTimeout != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.callerTimeout)
				defer cancel()
			}
			started := time.Now()
			var lookupDone <-chan struct{}
			var lookupDeadline time.Time
			resolver := httpResolverFunc(func(ctx context.Context, _, _ string) ([]netip.Addr, error) {
				lookupDone = ctx.Done()
				deadline, ok := ctx.Deadline()
				lookupDeadline = deadline
				if !ok {
					t.Fatal("DNS lookup has no deadline")
				}
				if deadline.After(time.Now().Add(httpStorageDialTimeout)) {
					t.Fatalf("DNS deadline %s exceeds the lookup timeout", deadline)
				}
				if tt.callerTimeout == 0 || tt.callerTimeout > httpStorageDialTimeout {
					if deadline.Before(started.Add(httpStorageDialTimeout)) {
						t.Fatalf("DNS deadline %s is earlier than the lookup timeout", deadline)
					}
				}
				return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
			})
			if _, err := resolveHTTPHost(ctx, resolver, "models.example"); err != nil {
				t.Fatalf("expected lookup to succeed: %v", err)
			}
			if tt.callerTimeout != 0 && tt.callerTimeout < httpStorageDialTimeout {
				callerDeadline, _ := ctx.Deadline()
				if !lookupDeadline.Equal(callerDeadline) {
					t.Fatalf("DNS deadline = %s, want caller deadline %s", lookupDeadline, callerDeadline)
				}
			}
			select {
			case <-lookupDone:
			default:
				t.Fatal("lookup context was not released")
			}
			if ctx.Err() != nil {
				t.Fatalf("lookup canceled the caller context: %v", ctx.Err())
			}
		})
	}
}

func TestResolveHTTPHostPreservesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := httpResolverFunc(func(ctx context.Context, _, _ string) ([]netip.Addr, error) {
		return nil, ctx.Err()
	})
	if _, err := resolveHTTPHost(ctx, resolver, "models.example"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected caller cancellation, got: %v", err)
	}
}
