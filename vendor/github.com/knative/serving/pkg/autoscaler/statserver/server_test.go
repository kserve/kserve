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

package statserver_test

import (
	"bytes"
	"encoding/gob"
	"net/url"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/gorilla/websocket"
	"github.com/knative/serving/pkg/autoscaler"
	stats "github.com/knative/serving/pkg/autoscaler/statserver"
	"golang.org/x/sync/errgroup"
)

func TestServerLifecycle(t *testing.T) {
	statsCh := make(chan *autoscaler.StatMessage)
	server := stats.NewTestServer(statsCh)

	eg := errgroup.Group{}
	eg.Go(func() error {
		return server.ListenAndServe()
	})

	server.ListenAddr()
	server.Shutdown(time.Second)

	if err := eg.Wait(); err != nil {
		t.Fatal("ListenAndServe failed.", err)
	}
}

func TestStatsReceived(t *testing.T) {
	statsCh := make(chan *autoscaler.StatMessage)
	server := stats.NewTestServer(statsCh)

	defer server.Shutdown(0)
	go server.ListenAndServe()

	statSink := dialOk(server.ListenAddr(), t)

	assertReceivedOk(newStatMessage("test-namespace/test-revision", "activator1", 2.1, 51), statSink, statsCh, t)
	assertReceivedOk(newStatMessage("test-namespace/test-revision2", "activator2", 2.2, 30), statSink, statsCh, t)

	closeSink(statSink, t)
}

func TestServerShutdown(t *testing.T) {
	statsCh := make(chan *autoscaler.StatMessage)
	server := stats.NewTestServer(statsCh)

	go server.ListenAndServe()

	listenAddr := server.ListenAddr()
	statSink := dialOk(listenAddr, t)

	assertReceivedOk(newStatMessage("test-namespace/test-revision", "activator1", 2.1, 51), statSink, statsCh, t)

	server.Shutdown(time.Second)
	// We own the channel.
	close(statsCh)

	// Send a statistic to the server
	send(statSink, newStatMessage("test-namespace/test-revision2", "activator2", 2.2, 30), t)

	// Check the statistic was not received
	_, ok := <-statsCh
	if ok {
		t.Fatal("Received statistic after shutdown")
	}

	// Check connection has been closed with a close control message with a "service restart" close code
	if _, _, err := statSink.NextReader(); err == nil {
		t.Fatal("Connection not closed")
	} else {
		err, ok := err.(*websocket.CloseError)
		if !ok {
			t.Fatal("CloseError not received")
		}
		if err.Code != 1012 {
			t.Fatalf("CloseError with unexpected close code %d received", err.Code)
		}
	}

	// Check that new connections are refused with some error
	if _, err := dial(listenAddr, t); err == nil {
		t.Fatal("Connection not refused")
	}

	closeSink(statSink, t)
}

func TestServerDoesNotLeakGoroutines(t *testing.T) {
	statsCh := make(chan *autoscaler.StatMessage)
	server := stats.NewTestServer(statsCh)

	go server.ListenAndServe()

	originalGoroutines := runtime.NumGoroutine()

	listenAddr := server.ListenAddr()
	statSink := dialOk(listenAddr, t)

	assertReceivedOk(newStatMessage("test-namespace/test-revision", "activator1", 2.1, 51), statSink, statsCh, t)

	closeSink(statSink, t)

	// Check the number of goroutines eventually reduces to the number there were before the connection was created
	for i := 1000; i >= 0; i-- {
		currentGoRoutines := runtime.NumGoroutine()
		if currentGoRoutines <= originalGoroutines {
			break
		}
		time.Sleep(5 * time.Millisecond)
		if i == 0 {
			t.Fatalf("Current number of goroutines %d is not equal to the original number %d", currentGoRoutines, originalGoroutines)
		}
	}

	server.Shutdown(time.Second)
}

func newStatMessage(revKey string, podName string, averageConcurrentRequests float64, requestCount int32) *autoscaler.StatMessage {
	return &autoscaler.StatMessage{
		Key: revKey,
		Stat: autoscaler.Stat{
			PodName:                   podName,
			AverageConcurrentRequests: averageConcurrentRequests,
			RequestCount:              requestCount,
		},
	}
}

func assertReceivedOk(sm *autoscaler.StatMessage, statSink *websocket.Conn, statsCh <-chan *autoscaler.StatMessage, t *testing.T) bool {
	send(statSink, sm, t)
	recv, ok := <-statsCh
	if !ok {
		t.Fatalf("statistic not received")
	}
	if recv.Stat.Time == nil {
		t.Fatalf("Stat time is nil")
	}
	ignoreTimeField := cmpopts.IgnoreFields(autoscaler.StatMessage{}, "Stat.Time")
	if !cmp.Equal(sm, recv, ignoreTimeField) {
		t.Fatalf("StatMessage mismatch: diff (-got, +want) %s", cmp.Diff(recv, sm, ignoreTimeField))
	}
	return true
}

func dialOk(serverURL string, t *testing.T) *websocket.Conn {
	statSink, err := dial(serverURL, t)
	if err != nil {
		t.Fatal("Dial failed:", err)
	}
	return statSink
}

func dial(serverURL string, t *testing.T) (*websocket.Conn, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Scheme = "ws"

	dialer := &websocket.Dialer{
		HandshakeTimeout: time.Second,
	}
	statSink, _, err := dialer.Dial(u.String(), nil)
	return statSink, err
}

func send(statSink *websocket.Conn, sm *autoscaler.StatMessage, t *testing.T) {
	var b bytes.Buffer
	enc := gob.NewEncoder(&b)

	if err := enc.Encode(sm); err != nil {
		t.Fatal("Failed to encode data from stats channel:", err)
	}
	if err := statSink.WriteMessage(websocket.BinaryMessage, b.Bytes()); err != nil {
		t.Fatal("Failed to write to stat sink:", err)
	}
}

func closeSink(statSink *websocket.Conn, t *testing.T) {
	if err := statSink.Close(); err != nil {
		t.Fatal("Failed to close", err)
	}
}
