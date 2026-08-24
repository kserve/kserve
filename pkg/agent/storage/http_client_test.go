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
	"net"
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

type recordingHTTPDialer struct {
	addresses []string
}

func (d *recordingHTTPDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.addresses = append(d.addresses, address)
	return nil, errors.New("test dial stopped")
}

func TestRestrictedHTTPDialContextPinsValidatedAddress(t *testing.T) {
	dialer := &recordingHTTPDialer{}
	dial := restrictedHTTPDialContext(dialer, staticHTTPResolver{
		addrs: []netip.Addr{netip.MustParseAddr("93.184.216.34")},
	})

	_, err := dial(context.Background(), "tcp", "models.example:443")
	if err == nil {
		t.Fatal("expected test dial to stop with an error")
	}
	if len(dialer.addresses) != 1 || dialer.addresses[0] != "93.184.216.34:443" {
		t.Fatalf("dialed addresses = %v, want the validated IP address", dialer.addresses)
	}
}

func TestRestrictedHTTPDialContextRejectsReboundAddress(t *testing.T) {
	dialer := &recordingHTTPDialer{}
	dial := restrictedHTTPDialContext(dialer, staticHTTPResolver{
		addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
	})

	_, err := dial(context.Background(), "tcp", "models.example:80")
	if err == nil || !strings.Contains(err.Error(), "blocked unsafe HTTP(S) storage destination") {
		t.Fatalf("expected unsafe destination error, got: %v", err)
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("dialed unsafe addresses: %v", dialer.addresses)
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
}
