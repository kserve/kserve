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
package handler

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/knative/pkg/test/helpers"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	. "github.com/knative/pkg/logging/testing"
	"github.com/knative/serving/pkg/activator"
	"github.com/knative/serving/pkg/activator/util"
	nv1a1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/knative/serving/pkg/network"
	"github.com/knative/serving/pkg/queue"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	wantBody      = "everything good!"
	testNamespace = "real-namespace"
	testRevName   = "real-name"
)

func stubRevisionGetter(revID activator.RevisionID) (*v1alpha1.Revision, error) {
	if revID.Namespace != testNamespace {
		return nil, &k8serrors.StatusError{
			ErrStatus: metav1.Status{
				Status:  metav1.StatusFailure,
				Code:    http.StatusNotFound,
				Reason:  metav1.StatusReasonNotFound,
				Message: fmt.Sprintf("not found"),
			}}
	}
	revision := &v1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: revID.Namespace,
			Name:      revID.Name,
			Labels: map[string]string{
				serving.ConfigurationLabelKey: "config-" + revID.Name,
				serving.ServiceLabelKey:       "service-" + revID.Name,
			},
		},
		Spec: v1alpha1.RevisionSpec{
			RevisionSpec: v1beta1.RevisionSpec{
				ContainerConcurrency: 1,
			},
		},
	}
	return revision, nil
}

func sksErrorGetter(string, string) (*nv1a1.ServerlessService, error) {
	return nil, errors.New("no luck in this land")
}

func stubSKSGetter(namespace, name string) (*nv1a1.ServerlessService, error) {
	return &nv1a1.ServerlessService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Status: nv1a1.ServerlessServiceStatus{
			// Randomize the test.
			PrivateServiceName: helpers.AppendRandomString(name),
			ServiceName:        helpers.AppendRandomString(name),
		},
	}, nil
}

func erroringServiceGetter(string, string) (*corev1.Service, error) {
	return nil, errors.New("wish this call succeeded")
}

func incorrectServiceGetter(namespace, name string) (*corev1.Service, error) {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name: "julio",
				Port: 8080,
			}},
		}}, nil
}

func stubServiceGetter(namespace, name string) (*corev1.Service, error) {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name: "http",
				Port: 8080,
			}},
		}}, nil
}

func errMsg(msg string) string {
	return fmt.Sprintf("Error getting active endpoint: %v\n", msg)
}

func goodEndpointsGetter(*nv1a1.ServerlessService) (int, error) {
	return 1000, nil
}

func brokenEndpointsCountGetter(*nv1a1.ServerlessService) (int, error) {
	return 0, errors.New("some error")
}

func TestActivationHandler(t *testing.T) {
	defer ClearAll()

	tests := []struct {
		label           string
		namespace       string
		name            string
		wantBody        string
		wantCode        int
		wantErr         error
		probeErr        error
		probeCode       int
		probeResp       []string
		gpc             int
		endpointsGetter activator.EndpointsCountGetter
		sksGetter       activator.SKSGetter
		svcGetter       activator.ServiceGetter
		reporterCalls   []reporterCall
	}{{
		label:           "active endpoint",
		namespace:       testNamespace,
		name:            testRevName,
		wantBody:        "everything good!",
		wantCode:        http.StatusOK,
		wantErr:         nil,
		endpointsGetter: goodEndpointsGetter,
		reporterCalls: []reporterCall{{
			Op:         "ReportRequestCount",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusOK,
			Attempts:   2, // probe + request
			Value:      1,
		}, {
			Op:         "ReportResponseTime",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusOK,
		}},
		gpc: 1,
	}, {
		label:           "slowly active endpoint",
		namespace:       testNamespace,
		name:            testRevName,
		wantBody:        "everything good!",
		wantCode:        http.StatusOK,
		wantErr:         nil,
		probeResp:       []string{activator.Name, queue.Name},
		endpointsGetter: goodEndpointsGetter,
		reporterCalls: []reporterCall{{
			Op:         "ReportRequestCount",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusOK,
			Attempts:   3, // probe + probe + request
			Value:      1,
		}, {
			Op:         "ReportResponseTime",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusOK,
		}},
		gpc: 2,
	}, {
		label:           "active endpoint with missing count header",
		namespace:       testNamespace,
		name:            testRevName,
		wantBody:        "everything good!",
		wantCode:        http.StatusOK,
		wantErr:         nil,
		endpointsGetter: goodEndpointsGetter,
		reporterCalls: []reporterCall{{
			Op:         "ReportRequestCount",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusOK,
			Attempts:   1,
			Value:      1,
		}, {
			Op:         "ReportResponseTime",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusOK,
		}},
	}, {
		label:           "no active endpoint",
		namespace:       "fake-namespace",
		name:            "fake-name",
		wantBody:        errMsg("not found"),
		wantCode:        http.StatusNotFound,
		wantErr:         nil,
		endpointsGetter: goodEndpointsGetter,
		reporterCalls:   nil,
	}, {
		label:           "active endpoint (probe failure)",
		namespace:       testNamespace,
		name:            testRevName,
		probeErr:        errors.New("probe error"),
		wantCode:        http.StatusInternalServerError,
		endpointsGetter: goodEndpointsGetter,
		gpc:             1,
		reporterCalls: []reporterCall{{
			Op:         "ReportRequestCount",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusInternalServerError,
			Attempts:   1,
			Value:      1,
		}, {
			Op:         "ReportResponseTime",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusInternalServerError,
		}},
	}, {
		label:           "active endpoint (probe 500)",
		namespace:       testNamespace,
		name:            testRevName,
		probeCode:       http.StatusServiceUnavailable,
		wantCode:        http.StatusInternalServerError,
		endpointsGetter: goodEndpointsGetter,
		gpc:             1,
		reporterCalls: []reporterCall{{
			Op:         "ReportRequestCount",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusInternalServerError,
			Attempts:   1,
			Value:      1,
		}, {
			Op:         "ReportResponseTime",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusInternalServerError,
		}},
	}, {
		label:           "request error",
		namespace:       testNamespace,
		name:            testRevName,
		wantBody:        "",
		wantCode:        http.StatusBadGateway,
		wantErr:         errors.New("request error"),
		endpointsGetter: goodEndpointsGetter,
		reporterCalls: []reporterCall{{
			Op:         "ReportRequestCount",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusBadGateway,
			Attempts:   1,
			Value:      1,
		}, {
			Op:         "ReportResponseTime",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusBadGateway,
		}},
	}, {
		label:           "invalid number of attempts",
		namespace:       testNamespace,
		name:            testRevName,
		wantBody:        "everything good!",
		wantCode:        http.StatusOK,
		wantErr:         nil,
		endpointsGetter: goodEndpointsGetter,
		reporterCalls: []reporterCall{{
			Op:         "ReportRequestCount",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusOK,
			Attempts:   1,
			Value:      1,
		}, {
			Op:         "ReportResponseTime",
			Namespace:  testNamespace,
			Revision:   testRevName,
			Service:    "service-real-name",
			Config:     "config-real-name",
			StatusCode: http.StatusOK,
		}},
	}, {
		label:           "broken get SKS",
		namespace:       testNamespace,
		name:            testRevName,
		wantBody:        errMsg("no luck in this land"),
		wantCode:        http.StatusInternalServerError,
		wantErr:         nil,
		endpointsGetter: goodEndpointsGetter,
		sksGetter:       sksErrorGetter,
		reporterCalls:   nil,
	}, {
		label:           "k8s svc incorrectly spec'd",
		namespace:       testNamespace,
		name:            testRevName,
		wantBody:        errMsg("revision needs external HTTP port"),
		wantCode:        http.StatusInternalServerError,
		wantErr:         nil,
		endpointsGetter: goodEndpointsGetter,
		svcGetter:       incorrectServiceGetter,
		reporterCalls:   nil,
	}, {
		label:           "broken get k8s svc",
		namespace:       testNamespace,
		name:            testRevName,
		wantBody:        errMsg("wish this call succeeded"),
		wantCode:        http.StatusInternalServerError,
		wantErr:         nil,
		endpointsGetter: goodEndpointsGetter,
		svcGetter:       erroringServiceGetter,
		reporterCalls:   nil,
	}, {
		label:           "broken GetEndpoints",
		namespace:       testNamespace,
		name:            testRevName,
		wantBody:        "",
		wantCode:        http.StatusInternalServerError,
		wantErr:         nil,
		endpointsGetter: brokenEndpointsCountGetter,
		reporterCalls:   nil,
	}}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			rt := util.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get(network.ProbeHeaderName) != "" {
					if test.probeErr != nil {
						return nil, test.probeErr
					}
					fake := httptest.NewRecorder()
					fake.WriteHeader(test.probeCode)
					probeResp := queue.Name
					if len(test.probeResp) > 0 {
						probeResp = test.probeResp[0]
						test.probeResp = test.probeResp[1:]
					}
					fake.WriteString(probeResp)
					return fake.Result(), nil
				}
				if test.wantErr != nil {
					return nil, test.wantErr
				}

				fake := httptest.NewRecorder()

				fake.WriteHeader(http.StatusOK)
				fake.WriteString(wantBody)
				return fake.Result(), nil
			})

			reporter := &fakeReporter{}
			params := queue.BreakerParams{QueueDepth: 1000, MaxConcurrency: 1000, InitialCapacity: 0}
			throttlerParams := activator.ThrottlerParams{
				BreakerParams: params,
				Logger:        TestLogger(t),
				GetRevision:   stubRevisionGetter,
				GetEndpoints:  test.endpointsGetter,
				GetSKS:        stubSKSGetter,
			}
			handler := ActivationHandler{
				Transport:     rt,
				Logger:        TestLogger(t),
				Reporter:      reporter,
				Throttler:     activator.NewThrottler(throttlerParams),
				GetProbeCount: test.gpc,
				GetRevision:   stubRevisionGetter,
				GetService:    stubServiceGetter,
				GetSKS:        stubSKSGetter,
			}
			if test.sksGetter != nil {
				handler.GetSKS = test.sksGetter
			}
			if test.svcGetter != nil {
				handler.GetService = test.svcGetter
			}

			resp := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
			req.Header.Set(activator.RevisionHeaderNamespace, test.namespace)
			req.Header.Set(activator.RevisionHeaderName, test.name)

			handler.ServeHTTP(resp, req)

			if resp.Code != test.wantCode {
				t.Errorf("Unexpected response status. Want %d, got %d", test.wantCode, resp.Code)
			}

			gotBody, _ := ioutil.ReadAll(resp.Body)
			if string(gotBody) != test.wantBody {
				t.Errorf("Unexpected response body. Response body %q, want %q", gotBody, test.wantBody)
			}

			if diff := cmp.Diff(test.reporterCalls, reporter.calls, ignoreDurationOption); diff != "" {
				t.Errorf("Reporting calls are different (-want, +got) = %v", diff)
			}
		})
	}

}

// Make sure we return http internal server error when the Breaker is overflowed
func TestActivationHandler_Overflow(t *testing.T) {
	const (
		wantedSuccess = 20
		wantedFailure = 1
		requests      = wantedSuccess + wantedFailure
	)
	respCh := make(chan *httptest.ResponseRecorder, requests)
	// overall max 20 requests in the Breaker
	breakerParams := queue.BreakerParams{QueueDepth: 10, MaxConcurrency: 10, InitialCapacity: 10}

	namespace := testNamespace
	revName := testRevName

	throttler := getThrottler(breakerParams, t)

	lockerCh := make(chan struct{})
	handler := getHandler(throttler, lockerCh, t)
	sendRequests(requests, namespace, revName, respCh, handler)

	assertResponses(wantedSuccess, wantedFailure, requests, lockerCh, respCh, t)
}

// Make sure if one breaker is overflowed, the requests to other revisions are still served
func TestActivationHandler_OverflowSeveralRevisions(t *testing.T) {
	const (
		wantedSuccess   = 40
		wantedFailure   = 2
		overallRequests = wantedSuccess + wantedFailure
		revisions       = 2
	)

	respCh := make(chan *httptest.ResponseRecorder, overallRequests)
	breakerParams := queue.BreakerParams{QueueDepth: 10, MaxConcurrency: 10, InitialCapacity: 10}
	throttler := getThrottler(breakerParams, t)
	lockerCh := make(chan struct{})

	for rev := 0; rev < revisions; rev++ {
		revName := fmt.Sprintf("%s-%d", testRevName, rev)

		handler := getHandler(throttler, lockerCh, t)

		requestCount := overallRequests / revisions
		sendRequests(requestCount, testNamespace, revName, respCh, handler)
	}
	assertResponses(wantedSuccess, wantedFailure, overallRequests, lockerCh, respCh, t)
}

func TestActivationHandler_ProxyHeader(t *testing.T) {
	breakerParams := queue.BreakerParams{QueueDepth: 10, MaxConcurrency: 10, InitialCapacity: 10}
	namespace, revName := testNamespace, testRevName

	interceptCh := make(chan *http.Request, 1)
	rt := util.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		interceptCh <- r
		fake := httptest.NewRecorder()
		return fake.Result(), nil
	})
	throttler := getThrottler(breakerParams, t)

	handler := ActivationHandler{
		Transport:   rt,
		Logger:      TestLogger(t),
		Reporter:    &fakeReporter{},
		Throttler:   throttler,
		GetRevision: stubRevisionGetter,
		GetService:  stubServiceGetter,
		GetSKS:      stubSKSGetter,
	}

	writer := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
	req.Header.Set(activator.RevisionHeaderNamespace, namespace)
	req.Header.Set(activator.RevisionHeaderName, revName)
	handler.ServeHTTP(writer, req)

	select {
	case httpReq := <-interceptCh:
		if got := httpReq.Header.Get(network.ProxyHeaderName); got != activator.Name {
			t.Errorf("Header '%s' does not have the expected value. Want = '%s', got = '%s'.", network.ProxyHeaderName, activator.Name, got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for a request to be intercepted")
	}
}

// sendRequests sends `count` concurrent requests via the given handler and writes
// the recorded responses to the `respCh`.
func sendRequests(count int, namespace, revName string, respCh chan *httptest.ResponseRecorder, handler ActivationHandler) {
	for i := 0; i < count; i++ {
		go func() {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
			req.Header.Set(activator.RevisionHeaderNamespace, namespace)
			req.Header.Set(activator.RevisionHeaderName, revName)
			handler.ServeHTTP(resp, req)
			respCh <- resp
		}()
	}
}

// getThrottler returns a fully setup Throttler with some sensible defaults for tests.
func getThrottler(breakerParams queue.BreakerParams, t *testing.T) *activator.Throttler {
	endpointsGetter := func(*nv1a1.ServerlessService) (int, error) {
		// Since revisions have a concurrency of 1, this will cause the very same capacity
		// as being set initially.
		return breakerParams.InitialCapacity, nil
	}
	throttlerParams := activator.ThrottlerParams{
		BreakerParams: breakerParams,
		Logger:        TestLogger(t),
		GetRevision:   stubRevisionGetter,
		GetEndpoints:  endpointsGetter,
		GetSKS:        stubSKSGetter,
	}
	throttler := activator.NewThrottler(throttlerParams)
	return throttler
}

// getHandler returns an already setup ActivationHandler. The roundtripper is controlled
// via the given `lockerCh`.
func getHandler(throttler *activator.Throttler, lockerCh chan struct{}, t *testing.T) ActivationHandler {
	rt := util.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		// Allows only one request at a time until read from.
		lockerCh <- struct{}{}

		fake := httptest.NewRecorder()
		fake.WriteHeader(http.StatusOK)
		fake.WriteString(wantBody)

		return fake.Result(), nil
	})
	handler := ActivationHandler{
		Transport:   rt,
		Logger:      TestLogger(t),
		Reporter:    &fakeReporter{},
		Throttler:   throttler,
		GetRevision: stubRevisionGetter,
		GetService:  stubServiceGetter,
		GetSKS:      stubSKSGetter,
	}
	return handler
}

func assertResponses(wantedSuccess, wantedFailure, overallRequests int, lockerCh chan struct{}, respCh chan *httptest.ResponseRecorder, t *testing.T) {
	t.Helper()

	const channelTimeout = 3 * time.Second
	var (
		successCode = http.StatusOK
		failureCode = http.StatusServiceUnavailable

		succeeded int
		failed    int
	)

	processResponse := func(chan *httptest.ResponseRecorder) {
		select {
		case resp := <-respCh:
			bodyBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read body: %v", err)
			}
			gotBody := strings.TrimSpace(string(bodyBytes))

			switch resp.Code {
			case successCode:
				succeeded++
				if gotBody != wantBody {
					t.Errorf("response body = %q, want: %q", gotBody, wantBody)
				}
			case failureCode:
				failed++
				if gotBody != activator.ErrActivatorOverload.Error() {
					t.Errorf("error message = %q, want: %q", gotBody, activator.ErrActivatorOverload.Error())
				}
			default:
				t.Errorf("http response code = %d, want: %d or %d", resp.Code, successCode, failureCode)
			}
		case <-time.After(channelTimeout):
			t.Fatalf("Timed out waiting for a request to be returned")
		}
	}

	// The failures will arrive first, because we block other requests from being executed
	for i := 0; i < wantedFailure; i++ {
		processResponse(respCh)
	}

	for i := 0; i < wantedSuccess; i++ {
		// All of the success requests are locked via the lockerCh.
		select {
		case <-lockerCh:
			// All good.
		case <-time.After(channelTimeout):
			t.Fatalf("Timed out waiting for a request to reach the RoundTripper")
		}
		processResponse(respCh)
	}

	if wantedFailure != failed {
		t.Errorf("failed request count = %d, want: %d", failed, wantedFailure)
	}
	if succeeded != wantedSuccess {
		t.Errorf("successful request count = %d, want: %d", succeeded, wantedSuccess)
	}

}

var ignoreDurationOption = cmpopts.IgnoreFields(reporterCall{}, "Duration")

type reporterCall struct {
	Op         string
	Namespace  string
	Service    string
	Config     string
	Revision   string
	StatusCode int
	Attempts   int
	Value      int64
	Duration   time.Duration
}

type fakeReporter struct {
	calls []reporterCall
	mux   sync.Mutex
}

func (f *fakeReporter) ReportRequestCount(ns, service, config, rev string, responseCode, numTries int, v int64) error {
	f.mux.Lock()
	defer f.mux.Unlock()
	f.calls = append(f.calls, reporterCall{
		Op:         "ReportRequestCount",
		Namespace:  ns,
		Service:    service,
		Config:     config,
		Revision:   rev,
		StatusCode: responseCode,
		Attempts:   numTries,
		Value:      v,
	})

	return nil
}

func (f *fakeReporter) ReportResponseTime(ns, service, config, rev string, responseCode int, d time.Duration) error {
	f.mux.Lock()
	defer f.mux.Unlock()
	f.calls = append(f.calls, reporterCall{
		Op:         "ReportResponseTime",
		Namespace:  ns,
		Service:    service,
		Config:     config,
		Revision:   rev,
		StatusCode: responseCode,
		Duration:   d,
	})

	return nil
}
