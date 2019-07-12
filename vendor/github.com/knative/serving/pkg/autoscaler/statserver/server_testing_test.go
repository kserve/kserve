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

package statserver

import (
	"net"

	"github.com/knative/serving/pkg/autoscaler"
	"go.uber.org/zap"
)

const testAddress = "127.0.0.1:0"

type TestServer struct {
	*Server
	listenAddr chan string
}

func NewTestServer(statsCh chan<- *autoscaler.StatMessage) *TestServer {
	return &TestServer{
		Server:     New(testAddress, statsCh, zap.NewNop().Sugar()),
		listenAddr: make(chan string, 1),
	}
}

// ListenAddr returns the address on which the server is listening. Blocks until ListenAndServe is called.
func (s *TestServer) ListenAddr() string {
	return <-s.listenAddr
}

func (s *TestServer) ListenAndServe() error {
	listener, err := s.listen()
	if err != nil {
		return err
	}
	return s.serve(&testListener{listener, s.listenAddr})
}

type testListener struct {
	net.Listener
	listenAddr chan string
}

func (t *testListener) Accept() (net.Conn, error) {
	t.listenAddr <- "http://" + t.Listener.Addr().String()
	return t.Listener.Accept()
}
