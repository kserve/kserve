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

package http

import (
	"bufio"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/knative/pkg/websocket"
)

var (
	_ http.Flusher        = (*ResponseRecorder)(nil)
	_ http.ResponseWriter = (*ResponseRecorder)(nil)
)

// ResponseRecorder is an implementation of http.ResponseWriter and http.Flusher
// that captures the response code and size.
type ResponseRecorder struct {
	ResponseCode int
	ResponseSize int32

	writer      http.ResponseWriter
	wroteHeader bool
	// hijacked is whether this connection has been hijacked
	// by a Handler with the Hijacker interface.
	// This is guarded by a mutex in the default implementation.
	// To emulate the same behavior, we will use an int32 and
	// access to this field only through atomic calls.
	hijacked int32
}

// NewResponseRecorder creates an http.ResponseWriter that captures the response code and size.
func NewResponseRecorder(w http.ResponseWriter, responseCode int) *ResponseRecorder {
	return &ResponseRecorder{
		writer:       w,
		ResponseCode: responseCode,
	}
}

// Flush flushes the buffer to the client.
func (rr *ResponseRecorder) Flush() {
	rr.writer.(http.Flusher).Flush()
}

// Hijack calls Hijack() on the wrapped http.ResponseWriter if it implements
// http.Hijacker interface, which is required for net/http/httputil/reverseproxy
// to handle connection upgrade/switching protocol.  Otherwise returns an error.
func (rr *ResponseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, rw, err := websocket.HijackIfPossible(rr.writer)
	if err != nil {
		atomic.StoreInt32(&rr.hijacked, 1)
	}
	return c, rw, err
}

// Header returns the header map that will be sent by WriteHeader.
func (rr *ResponseRecorder) Header() http.Header {
	return rr.writer.Header()
}

// Write writes the data to the connection as part of an HTTP reply.
func (rr *ResponseRecorder) Write(p []byte) (int, error) {
	atomic.AddInt32(&rr.ResponseSize, (int32)(len(p)))
	return rr.writer.Write(p)
}

// WriteHeader sends an HTTP response header with the provided status code.
func (rr *ResponseRecorder) WriteHeader(code int) {
	if rr.wroteHeader || atomic.LoadInt32(&rr.hijacked) == 1 {
		return
	}

	rr.writer.WriteHeader(code)
	rr.wroteHeader = true
	rr.ResponseCode = code
}
