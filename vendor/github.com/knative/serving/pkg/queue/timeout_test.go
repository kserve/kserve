/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package queue

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeToFirstByteTimeoutHandler(t *testing.T) {
	tests := []struct {
		name           string
		timeout        time.Duration
		handler        func(writeErrors chan error) http.Handler
		timeoutMessage string
		wantStatus     int
		wantBody       string
		wantWriteError bool
		wantPanic      bool
	}{{
		name:    "all good",
		timeout: 10 * time.Second,
		handler: func(writeErrors chan error) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("hi"))
			})
		},
		wantStatus: http.StatusOK,
		wantBody:   "hi",
	}, {
		name:    "timeout",
		timeout: 50 * time.Millisecond,
		handler: func(writeErrors chan error) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(100 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				_, werr := w.Write([]byte("hi"))
				writeErrors <- werr
			})
		},
		wantStatus:     http.StatusServiceUnavailable,
		wantBody:       defaultTimeoutBody,
		wantWriteError: true,
	}, {
		name:    "write then sleep",
		timeout: 10 * time.Millisecond,
		handler: func(writeErrors chan error) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				time.Sleep(50 * time.Millisecond)
				w.Write([]byte("hi"))
			})
		},
		wantStatus: http.StatusOK,
		wantBody:   "hi",
	}, {
		name:    "custom timeout message",
		timeout: 50 * time.Millisecond,
		handler: func(writeErrors chan error) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(100 * time.Millisecond)
				_, werr := w.Write([]byte("hi"))
				writeErrors <- werr
			})
		},
		timeoutMessage: "request timeout",
		wantStatus:     http.StatusServiceUnavailable,
		wantBody:       "request timeout",
		wantWriteError: true,
	}, {
		name:    "propagate panic",
		timeout: 50 * time.Millisecond,
		handler: func(writeErrors chan error) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic(http.ErrAbortHandler)
			})
		},
		wantStatus: http.StatusServiceUnavailable,
		wantBody:   "request timeout",
		wantPanic:  true,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/", nil)
			if err != nil {
				t.Fatal(err)
			}

			writeErrors := make(chan error, 1)
			rr := httptest.NewRecorder()
			handler := TimeToFirstByteTimeoutHandler(test.handler(writeErrors), test.timeout, test.timeoutMessage)

			defer func() {
				if test.wantPanic {
					if recovered := recover(); recovered != http.ErrAbortHandler {
						t.Error("Expected the handler to panic, but it didn't.")
					}
				}
			}()
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != test.wantStatus {
				t.Errorf("Handler returned wrong status code: got %v want %v", status, test.wantStatus)
			}

			if rr.Body.String() != test.wantBody {
				t.Errorf("Handler returned unexpected body: got %q want %q", rr.Body.String(), test.wantBody)
			}

			if test.wantWriteError {
				err := <-writeErrors
				if err != http.ErrHandlerTimeout {
					t.Errorf("Expected a timeout error, got %v", err)
				}
			}
		})
	}
}
