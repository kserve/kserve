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

package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTCPProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	serverAddr := server.Listener.Addr().String()

	// Connecting to the server should work
	if err := TCPProbe(serverAddr, 1*time.Second); err != nil {
		t.Errorf("Expected probe to succeed but it failed with %v", err)
	}

	// Close the server so probing fails afterwards
	server.Close()
	if err := TCPProbe(serverAddr, 1*time.Second); err == nil {
		t.Error("Expected probe to fail but it didn't")
	}
}
