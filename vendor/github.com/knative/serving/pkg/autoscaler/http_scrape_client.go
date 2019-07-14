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
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

type httpScrapeClient struct {
	httpClient *http.Client
}

func newHTTPScrapeClient(httpClient *http.Client) (*httpScrapeClient, error) {
	if httpClient == nil {
		return nil, errors.New("HTTP client must not be nil")
	}

	return &httpScrapeClient{
		httpClient: httpClient,
	}, nil
}

func (c *httpScrapeClient) Scrape(url string) (*Stat, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("GET request for URL %q returned HTTP status %v", url, resp.StatusCode)
	}

	return extractData(resp.Body)
}

func extractData(body io.Reader) (*Stat, error) {
	var parser expfmt.TextParser
	metricFamilies, err := parser.TextToMetricFamilies(body)
	if err != nil {
		return nil, fmt.Errorf("reading text format failed: %v", err)
	}

	now := time.Now()
	stat := Stat{
		Time: &now,
	}

	if pMetric := prometheusMetric(metricFamilies, "queue_average_concurrent_requests"); pMetric != nil {
		stat.AverageConcurrentRequests = *pMetric.Gauge.Value
	} else {
		return nil, errors.New("could not find value for queue_average_concurrent_requests in response")
	}

	if pMetric := prometheusMetric(metricFamilies, "queue_average_proxied_concurrent_requests"); pMetric != nil {
		stat.AverageProxiedConcurrentRequests = *pMetric.Gauge.Value
	} else {
		return nil, errors.New("could not find value for queue_average_proxied_concurrent_requests in response")
	}

	if pMetric := prometheusMetric(metricFamilies, "queue_operations_per_second"); pMetric != nil {
		stat.RequestCount = int32(*pMetric.Gauge.Value)
	} else {
		return nil, errors.New("could not find value for queue_operations_per_second in response")
	}

	if pMetric := prometheusMetric(metricFamilies, "queue_proxied_operations_per_second"); pMetric != nil {
		stat.ProxiedRequestCount = int32(*pMetric.Gauge.Value)
	} else {
		return nil, errors.New("could not find value for queue_proxied_operations_per_second in response")
	}

	return &stat, nil
}

// prometheusMetric returns the point of the first Metric of the MetricFamily
// with the given key from the given map. If there is no such MetricFamily or it
// has no Metrics, then returns nil.
func prometheusMetric(metricFamilies map[string]*dto.MetricFamily, key string) *dto.Metric {
	if metric, ok := metricFamilies[key]; ok && len(metric.Metric) > 0 {
		return metric.Metric[0]
	}

	return nil
}
