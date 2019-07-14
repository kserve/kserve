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

package autoscaler

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

const (
	testURL = "http://test-revision-metrics.test-namespace:9090/metrics"

	// TODO: Use Prometheus lib to generate the following text instead of using text format directly.
	testAverageConcurrencyContext = `# HELP queue_average_concurrent_requests Number of requests currently being handled by this pod
# TYPE queue_average_concurrent_requests gauge
queue_average_concurrent_requests{destination_namespace="test-namespace",destination_revision="test-revision",destination_pod="test-revision-1234"} 3.0
`
	testQPSContext = `# HELP queue_operations_per_second Number of requests received since last Stat
# TYPE queue_operations_per_second gauge
queue_operations_per_second{destination_namespace="test-namespace",destination_revision="test-revision",destination_pod="test-revision-1234"} 5
`
	testAverageProxiedConcurrenyContext = `# HELP queue_average_proxied_concurrent_requests Number of proxied requests currently being handled by this pod
# TYPE queue_average_proxied_concurrent_requests gauge
queue_average_proxied_concurrent_requests{destination_namespace="test-namespace",destination_revision="test-revision",destination_pod="test-revision-1234"} 2.0
`
	testProxiedQPSContext = `# HELP queue_proxied_operations_per_second Number of proxied requests received since last Stat
# TYPE queue_proxied_operations_per_second gauge
queue_proxied_operations_per_second{destination_namespace="test-namespace",destination_revision="test-revision",destination_pod="test-revision-1234"} 4
`
	testFullContext = testAverageConcurrencyContext + testQPSContext + testAverageProxiedConcurrenyContext + testProxiedQPSContext
)

func TestNewHTTPScrapeClient_ErrorCases(t *testing.T) {
	testCases := []struct {
		name        string
		client      *http.Client
		expectedErr string
	}{{
		name:        "Empty HTTP client",
		expectedErr: "HTTP client must not be nil",
	}}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newHTTPScrapeClient(test.client); err != nil {
				got := err.Error()
				want := test.expectedErr
				if got != want {
					t.Errorf("Got error message: %v. Want: %v", got, want)
				}
			} else {
				t.Errorf("Expected error from newHTTPScrapeClient, got nil")
			}
		})
	}
}

func TestHTTPScrapeClient_Scrape_HappyCase(t *testing.T) {
	hClient := newTestHTTPClient(getHTTPResponse(http.StatusOK, testFullContext), nil)
	sClient, err := newHTTPScrapeClient(hClient)
	if err != nil {
		t.Fatalf("newHTTPScrapeClient = %v, want no error", err)
	}

	// Scrape will set a timestamp bigger than this.
	now := time.Now()
	stat, err := sClient.Scrape(testURL)
	if err != nil {
		t.Errorf("scrapeViaURL = %v, want no error", err)
	}
	if stat.AverageConcurrentRequests != 3.0 {
		t.Errorf("stat.AverageConcurrentRequests = %v, want 3.0", stat.AverageConcurrentRequests)
	}
	if stat.RequestCount != 5 {
		t.Errorf("stat.RequestCount = %v, want 5", stat.RequestCount)
	}
	if stat.AverageProxiedConcurrentRequests != 2.0 {
		t.Errorf("stat.AverageProxiedConcurrency = %v, want 2.0", stat.AverageProxiedConcurrentRequests)
	}
	if stat.ProxiedRequestCount != 4 {
		t.Errorf("stat.ProxiedCount = %v, want 4", stat.ProxiedRequestCount)
	}
	if stat.Time.Before(now) {
		t.Errorf("stat.Time=%v, want bigger than %v", stat.Time, now)
	}
}

func TestHTTPScrapeClient_Scrape_ErrorCases(t *testing.T) {
	testCases := []struct {
		name            string
		responseCode    int
		responseErr     error
		responseContext string
		expectedErr     string
	}{{
		name:         "Non 200 return code",
		responseCode: http.StatusForbidden,
		expectedErr:  `GET request for URL "http://test-revision-metrics.test-namespace:9090/metrics" returned HTTP status 403`,
	}, {
		name:         "Error got when sending request",
		responseCode: http.StatusOK,
		responseErr:  errors.New("upstream closed"),
		expectedErr:  "Get http://test-revision-metrics.test-namespace:9090/metrics: upstream closed",
	}, {
		name:            "Bad response context format",
		responseCode:    http.StatusOK,
		responseContext: "bad context",
		expectedErr:     "reading text format failed: text format parsing error in line 1: unexpected end of input stream",
	}, {
		name:            "Missing average concurrency",
		responseCode:    http.StatusOK,
		responseContext: testQPSContext + testAverageProxiedConcurrenyContext + testProxiedQPSContext,
		expectedErr:     "could not find value for queue_average_concurrent_requests in response",
	}, {
		name:            "Missing QPS",
		responseCode:    http.StatusOK,
		responseContext: testAverageConcurrencyContext + testAverageProxiedConcurrenyContext + testProxiedQPSContext,
		expectedErr:     "could not find value for queue_operations_per_second in response",
	}, {
		name:            "Missing average proxied concurrency",
		responseCode:    http.StatusOK,
		responseContext: testAverageConcurrencyContext + testQPSContext + testProxiedQPSContext,
		expectedErr:     "could not find value for queue_average_proxied_concurrent_requests in response",
	}, {
		name:            "Missing proxied QPS",
		responseCode:    http.StatusOK,
		responseContext: testAverageConcurrencyContext + testQPSContext + testAverageProxiedConcurrenyContext,
		expectedErr:     "could not find value for queue_proxied_operations_per_second in response",
	}}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			hClient := newTestHTTPClient(getHTTPResponse(test.responseCode, test.responseContext), test.responseErr)
			sClient, err := newHTTPScrapeClient(hClient)
			if err != nil {
				t.Errorf("newHTTPScrapeClient=%v, want no error", err)
			}
			if _, err := sClient.Scrape(testURL); err != nil {
				if err.Error() != test.expectedErr {
					t.Errorf("Got error message: %q, want: %q", err.Error(), test.expectedErr)
				}
			} else {
				t.Errorf("Expected error from newServiceScraperWithClient, got nil")
			}
		})
	}
}

func getHTTPResponse(statusCode int, context string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       ioutil.NopCloser(bytes.NewBufferString(context)),
	}
}

type fakeRoundTripper struct {
	response      *http.Response
	responseError error
}

func (frt fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return frt.response, frt.responseError
}

func newTestHTTPClient(response *http.Response, err error) *http.Client {
	return &http.Client{
		Transport: fakeRoundTripper{
			response:      response,
			responseError: err,
		},
	}
}
