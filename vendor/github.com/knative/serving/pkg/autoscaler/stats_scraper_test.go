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
	"testing"

	"github.com/knative/serving/pkg/apis/serving"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

const (
	testRevision  = "test-revision"
	testService   = "test-revision-metrics"
	testNamespace = "test-namespace"
	testKPAKey    = "test-namespace/test-revision"
)

var (
	testStats = []*Stat{
		&Stat{
			AverageConcurrentRequests:        3.0,
			AverageProxiedConcurrentRequests: 2.0,
			RequestCount:                     5,
			ProxiedRequestCount:              4,
		},
	}
)

func TestNewServiceScraperWithClient_HappyCase(t *testing.T) {
	client := newTestScrapeClient(testStats, nil)
	if scraper, err := serviceScraperForTest(client); err != nil {
		t.Fatalf("newServiceScraperWithClient=%v, want no error", err)
	} else {
		if scraper.url != testURL {
			t.Errorf("scraper.url=%v, want %v", scraper.url, testURL)
		}
		if scraper.metricKey != testKPAKey {
			t.Errorf("scraper.metricKey=%v, want %v", scraper.metricKey, testKPAKey)
		}
	}
}

func TestNewServiceScraperWithClient_ErrorCases(t *testing.T) {
	metric := getTestMetric()
	invalidMetric := getTestMetric()
	invalidMetric.Labels = map[string]string{}
	client := newTestScrapeClient(testStats, nil)
	lister := kubeInformer.Core().V1().Endpoints().Lister()
	testCases := []struct {
		name        string
		metric      *Metric
		client      scrapeClient
		lister      corev1listers.EndpointsLister
		expectedErr string
	}{{
		name:        "Empty Decider",
		client:      client,
		lister:      lister,
		expectedErr: "metric must not be nil",
	}, {
		name:        "Missing revision label in Decider",
		metric:      invalidMetric,
		client:      client,
		lister:      lister,
		expectedErr: "no Revision label found for Metric test-revision",
	}, {
		name:        "Empty scrape client",
		metric:      metric,
		lister:      lister,
		expectedErr: "scrape client must not be nil",
	}, {
		name:        "Empty lister",
		metric:      metric,
		client:      client,
		expectedErr: "endpoints lister must not be nil",
	}}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newServiceScraperWithClient(test.metric, test.lister, test.client); err != nil {
				got := err.Error()
				want := test.expectedErr
				if got != want {
					t.Errorf("Got error message: %v. Want: %v", got, want)
				}
			} else {
				t.Errorf("Expected error from CreateNewServiceScraper, got nil")
			}
		})
	}
}

func TestScrape_HappyCase(t *testing.T) {
	client := newTestScrapeClient(testStats, nil)
	scraper, err := serviceScraperForTest(client)
	if err != nil {
		t.Fatalf("newServiceScraperWithClient=%v, want no error", err)
	}

	// Make an Endpoints with 2 pods.
	createEndpoints(addIps(makeEndpoints(), 2))
	got, err := scraper.Scrape()
	if err != nil {
		t.Fatalf("unexpected error from scraper.Scrape(): %v", err)
	}

	if got.Key != testKPAKey {
		t.Errorf("StatMessage.Key=%v, want %v", got.Key, testKPAKey)
	}
	if got.Stat.PodName != scraperPodName {
		t.Errorf("StatMessage.Stat.PodName=%v, want %v", got.Stat.PodName, scraperPodName)
	}
	// 2 pods times 3.0
	if got.Stat.AverageConcurrentRequests != 6.0 {
		t.Errorf("StatMessage.Stat.AverageConcurrentRequests=%v, want %v",
			got.Stat.AverageConcurrentRequests, 6.0)
	}
	// 2 pods times 5
	if got.Stat.RequestCount != 10 {
		t.Errorf("StatMessage.Stat.RequestCount=%v, want %v", got.Stat.RequestCount, 10)
	}
	// 2 pods times 2.0
	if got.Stat.AverageProxiedConcurrentRequests != 4.0 {
		t.Errorf("StatMessage.Stat.AverageProxiedConcurrentRequests=%v, want %v",
			got.Stat.AverageProxiedConcurrentRequests, 4.0)
	}
	// 2 pods times 4
	if got.Stat.ProxiedRequestCount != 8 {
		t.Errorf("StatMessage.Stat.ProxiedCount=%v, want %v", got.Stat.ProxiedRequestCount, 8)
	}
}

func TestScrape_PopulateErrorFromScrapeClient(t *testing.T) {
	errMsg := "test"
	client := newTestScrapeClient(testStats, errors.New(errMsg))
	scraper, err := serviceScraperForTest(client)
	if err != nil {
		t.Fatalf("newServiceScraperWithClient=%v, want no error", err)
	}

	// Make an Endpoints with 2 pods.
	createEndpoints(addIps(makeEndpoints(), 2))

	if _, err := scraper.Scrape(); err != nil {
		if got, want := err.Error(), errMsg; got != want {
			t.Errorf("Got error message: %v. Want: %v", got, want)
		}
	} else {
		t.Error("Expected an error from scraper.Scrape() but got none")
	}

}

func TestScrape_DoNotScrapeIfNoPodsFound(t *testing.T) {
	client := newTestScrapeClient(testStats, nil)
	scraper, err := serviceScraperForTest(client)
	if err != nil {
		t.Fatalf("newServiceScraperWithClient=%v, want no error", err)
	}

	// Override the Endpoints with 0 pods.
	createEndpoints(addIps(makeEndpoints(), 0))

	stat, err := scraper.Scrape()
	if stat != nil {
		t.Error("Received unexpected StatMessage.")
	}
}

func serviceScraperForTest(sClient scrapeClient) (*ServiceScraper, error) {
	metric := getTestMetric()
	return newServiceScraperWithClient(metric, kubeInformer.Core().V1().Endpoints().Lister(), sClient)
}

func getTestMetric() *Metric {
	return &Metric{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testRevision,
			Labels: map[string]string{
				serving.RevisionLabelKey: testRevision,
			},
		},
	}
}

func newTestScrapeClient(stats []*Stat, err error) scrapeClient {
	return &fakeScrapeClient{
		stats: stats,
		err:   err,
	}
}

type fakeScrapeClient struct {
	i     int
	stats []*Stat
	err   error
}

// Scrape return the next item in the stats array of fakeScrapeClient.
func (c *fakeScrapeClient) Scrape(url string) (*Stat, error) {
	ans := c.stats[c.i]
	c.i = (c.i + 1) % len(c.stats)
	return ans, c.err
}
