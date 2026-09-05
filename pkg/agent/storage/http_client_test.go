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
	"net/http"
	"net/netip"
	"strings"
	"testing"
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
