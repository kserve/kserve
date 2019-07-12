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
	"bufio"
	"bytes"
	"net"
	"net/http"
	"testing"
)

// hijackable is a http.ResponseWriter that implements http.Hijacker
type hijackable struct {
	http.ResponseWriter
	w *bufio.ReadWriter
}

// Hijack implements http.Hijacker
func (h *hijackable) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, h.w, nil
}

func newHijackable() *hijackable {
	var b *bytes.Buffer
	return &hijackable{
		w: bufio.NewReadWriter(bufio.NewReader(b), bufio.NewWriter(b)),
	}
}

type notHijackable struct {
	http.ResponseWriter
}

func TestHijackIfPossible(t *testing.T) {
	h := newHijackable()
	for _, test := range []struct {
		name           string
		w              http.ResponseWriter
		wantErr        bool
		wantReadWriter *bufio.ReadWriter
	}{{
		name:           "Hijacker type",
		w:              h,
		wantErr:        false,
		wantReadWriter: h.w,
	}, {
		name:           "non-Hijacker type",
		w:              &notHijackable{},
		wantErr:        true,
		wantReadWriter: nil,
	}} {
		t.Run(test.name, func(t *testing.T) {
			_, w, err := HijackIfPossible(test.w)
			if test.wantErr == (err == nil) {
				t.Errorf("wantErr=%v, but got err=%v", test.wantErr, err)
			}
			if test.wantReadWriter != w {
				t.Errorf("Wanted ReadWriter %v got %v", test.wantReadWriter, w)
			}
		})
	}
}
