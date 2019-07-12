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

package websocket

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ktesting "github.com/knative/pkg/logging/testing"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/gorilla/websocket"
)

const propagationTimeout = 5 * time.Second

type inspectableConnection struct {
	nextReaderCalls      chan struct{}
	writeMessageCalls    chan struct{}
	closeCalls           chan struct{}
	setReadDeadlineCalls chan struct{}
	setPongHandlerCalls  chan struct{}

	nextReaderFunc func() (int, io.Reader, error)
}

func (c *inspectableConnection) WriteMessage(messageType int, data []byte) error {
	if c.writeMessageCalls != nil {
		c.writeMessageCalls <- struct{}{}
	}
	return nil
}

func (c *inspectableConnection) NextReader() (int, io.Reader, error) {
	if c.nextReaderCalls != nil {
		c.nextReaderCalls <- struct{}{}
	}
	return c.nextReaderFunc()
}

func (c *inspectableConnection) Close() error {
	if c.closeCalls != nil {
		c.closeCalls <- struct{}{}
	}
	return nil
}

func (c *inspectableConnection) SetReadDeadline(deadline time.Time) error {
	if c.setReadDeadlineCalls != nil {
		c.setReadDeadlineCalls <- struct{}{}
	}
	return nil
}

func (c *inspectableConnection) SetPongHandler(func(string) error) {
	if c.setPongHandlerCalls != nil {
		c.setPongHandlerCalls <- struct{}{}
	}
	return
}

// staticConnFactory returns a static connection, for example
// an inspectable connection.
func staticConnFactory(conn rawConnection) func() (rawConnection, error) {
	return func() (rawConnection, error) {
		return conn, nil
	}
}

// errConnFactory returns a static error.
func errConnFactory(err error) func() (rawConnection, error) {
	return func() (rawConnection, error) {
		return nil, err
	}
}

func TestRetriesWhileConnect(t *testing.T) {
	wantConnects := 2
	gotConnects := 0

	spy := &inspectableConnection{
		closeCalls:           make(chan struct{}, 1),
		setReadDeadlineCalls: make(chan struct{}, 1),
		setPongHandlerCalls:  make(chan struct{}, 1),
	}

	connFactory := func() (rawConnection, error) {
		gotConnects++
		if gotConnects == wantConnects {
			return spy, nil
		}
		return nil, errors.New("not yet")
	}
	conn := newConnection(connFactory, nil)

	conn.connect()
	conn.Shutdown()

	if gotConnects != wantConnects {
		t.Fatalf("Wanted %v retries. Got %v.", wantConnects, gotConnects)
	}

	// We want a readDeadline and a pongHandler to be set on the final connection.
	if got, want := len(spy.setReadDeadlineCalls), 1; got != want {
		t.Fatalf("Got %d 'SetReadDeadline' calls, want %d", got, want)
	}
	if got, want := len(spy.setPongHandlerCalls), 1; got != want {
		t.Fatalf("Got %d 'SetPongHandler' calls, want %d", got, want)
	}

	if len(spy.closeCalls) != 1 {
		t.Fatalf("Wanted 'Close' to be called once, but got %v", len(spy.closeCalls))
	}
}

func TestSendErrorOnNoConnection(t *testing.T) {
	want := ErrConnectionNotEstablished

	conn := &ManagedConnection{}
	got := conn.Send("test")

	if got != want {
		t.Fatalf("Wanted error to be %v, but it was %v.", want, got)
	}
}

func TestStatusOnNoConnection(t *testing.T) {
	want := ErrConnectionNotEstablished

	conn := &ManagedConnection{}
	got := conn.Status()

	if got != want {
		t.Fatalf("Wanted error to be %v, but it was %v.", want, got)
	}
}

func TestSendErrorOnEncode(t *testing.T) {
	spy := &inspectableConnection{
		writeMessageCalls: make(chan struct{}, 1),
	}
	conn := newConnection(staticConnFactory(spy), nil)
	conn.connect()
	// gob cannot encode nil values
	got := conn.Send(nil)

	if got == nil {
		t.Fatal("Expected an error but got none")
	}
	if len(spy.writeMessageCalls) != 0 {
		t.Fatalf("Expected 'WriteMessage' not to be called, but was called %v times", spy.writeMessageCalls)
	}
}

func TestSendMessage(t *testing.T) {
	spy := &inspectableConnection{
		writeMessageCalls: make(chan struct{}, 1),
	}
	conn := newConnection(staticConnFactory(spy), nil)
	conn.connect()

	if got := conn.Status(); got != nil {
		t.Errorf("Status() = %v, wanted nil", got)
	}

	if got := conn.Send("test"); got != nil {
		t.Fatalf("Expected no error but got: %+v", got)
	}
	if len(spy.writeMessageCalls) != 1 {
		t.Fatalf("Expected 'WriteMessage' to be called once, but was called %v times", spy.writeMessageCalls)
	}
}

func TestReceiveMessage(t *testing.T) {
	testMessage := "testmessage"

	spy := &inspectableConnection{
		writeMessageCalls: make(chan struct{}, 1),
		nextReaderCalls:   make(chan struct{}, 1),
		nextReaderFunc: func() (int, io.Reader, error) {
			return websocket.TextMessage, strings.NewReader(testMessage), nil
		},
	}

	messageChan := make(chan []byte, 1)
	conn := newConnection(staticConnFactory(spy), messageChan)
	conn.connect()
	go conn.keepalive()

	got := <-messageChan

	if string(got) != testMessage {
		t.Errorf("Received the wrong message, wanted %q, got %q", testMessage, string(got))
	}
}

func TestCloseClosesConnection(t *testing.T) {
	spy := &inspectableConnection{
		closeCalls: make(chan struct{}, 1),
	}
	conn := newConnection(staticConnFactory(spy), nil)
	conn.connect()
	conn.Shutdown()

	if len(spy.closeCalls) != 1 {
		t.Fatalf("Expected 'Close' to be called once, got %v", len(spy.closeCalls))
	}
}

func TestCloseIgnoresNoConnection(t *testing.T) {
	conn := &ManagedConnection{
		closeChan: make(chan struct{}, 1),
	}
	got := conn.Shutdown()

	if got != nil {
		t.Fatalf("Expected no error, got %v", got)
	}
}

func TestConnectFailureReturnsError(t *testing.T) {
	conn := newConnection(errConnFactory(ErrConnectionNotEstablished), nil)

	// Shorten the connection backoff duration for this test
	conn.connectionBackoff.Duration = 1 * time.Millisecond

	got := conn.connect()

	if got == nil {
		t.Fatal("Expected an error but got none")
	}
}

func TestKeepaliveWithNoConnectionReturnsError(t *testing.T) {
	conn := newConnection(nil, nil)
	got := conn.keepalive()

	if got == nil {
		t.Fatal("Expected an error but got none")
	}
}

func TestConnectLoopIsStopped(t *testing.T) {
	conn := newConnection(errConnFactory(errors.New("connection error")), nil)

	errorChan := make(chan error)
	go func() {
		errorChan <- conn.connect()
	}()

	conn.Shutdown()

	select {
	case err := <-errorChan:
		if err != errShuttingDown {
			t.Errorf("Wrong 'connect' error, got %v, want %v", err, errShuttingDown)
		}
	case <-time.After(propagationTimeout):
		t.Error("Timed out waiting for the keepalive loop to stop.")
	}
}

func TestKeepaliveLoopIsStopped(t *testing.T) {
	spy := &inspectableConnection{
		nextReaderFunc: func() (int, io.Reader, error) {
			return websocket.TextMessage, nil, nil
		},
	}
	conn := newConnection(staticConnFactory(spy), nil)
	conn.connect()

	errorChan := make(chan error)
	go func() {
		errorChan <- conn.keepalive()
	}()

	conn.Shutdown()

	select {
	case err := <-errorChan:
		if err != errShuttingDown {
			t.Errorf("Wrong 'keepalive' error, got %v, want %v", err, errShuttingDown)
		}
	case <-time.After(propagationTimeout):
		t.Error("Timed out waiting for the keepalive loop to stop.")
	}
}

func TestDoubleShutdown(t *testing.T) {
	spy := &inspectableConnection{
		closeCalls: make(chan struct{}, 2), // potentially allow 2 calls
	}
	conn := newConnection(staticConnFactory(spy), nil)
	conn.connect()
	conn.Shutdown()
	conn.Shutdown()

	if want, got := 1, len(spy.closeCalls); want != got {
		t.Errorf("Wrong 'Close' callcount, got %d, want %d", got, want)
	}
}

func TestDurableConnectionWhenConnectionBreaksDown(t *testing.T) {
	testPayload := "test"
	reconnectChan := make(chan struct{})

	upgrader := websocket.Upgrader{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// Waits for a message to be sent before dropping the connection.
		<-reconnectChan
		c.Close()
	}))
	defer s.Close()

	logger := ktesting.TestLogger(t)
	target := "ws" + strings.TrimPrefix(s.URL, "http")
	conn := NewDurableSendingConnection(target, logger)
	defer conn.Shutdown()

	for i := 0; i < 10; i++ {
		err := wait.PollImmediate(50*time.Millisecond, 5*time.Second, func() (bool, error) {
			if err := conn.Send(testPayload); err != nil {
				return false, nil
			}
			return true, nil
		})

		if err != nil {
			t.Errorf("Timed out trying to send a message: %v", err)
		}

		// Message successfully sent, instruct the server to drop the connection.
		reconnectChan <- struct{}{}
	}
}

func TestDurableConnectionSendsPingsRegularly(t *testing.T) {
	// Reset pongTimeout to something quite short.
	pongTimeout = 100 * time.Millisecond

	upgrader := websocket.Upgrader{}

	pingReceived := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		c.SetPingHandler(func(_ string) error {
			pingReceived <- struct{}{}
			return c.WriteMessage(websocket.PongMessage, []byte{})
		})

		for {
			_, _, err := c.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer s.Close()

	logger := ktesting.TestLogger(t)
	target := "ws" + strings.TrimPrefix(s.URL, "http")
	conn := NewDurableSendingConnection(target, logger)
	defer conn.Shutdown()

	// Wait for 5 pings to be received by the server.
	for i := 0; i < 5; i++ {
		<-pingReceived
	}
}
