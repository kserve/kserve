/*
Copyright 2022 The KServe Authors.

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

package main

import (
	logger "github.com/kserve/kserve/qpext"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

var testEnvVarVal = "something"

func setEnvVars(t *testing.T) {
	for _, key := range EnvVars {
		t.Setenv(key, testEnvVarVal)
	}
}

func TestGetServerlessLabelVals(t *testing.T) {
	setEnvVars(t)
	labelVals := getServerlessLabelVals()
	for idx, val := range labelVals {
		assert.Equal(t, os.Getenv(EnvVars[idx]), val)
	}
}

func TestAddServerlessLabels(t *testing.T) {
	testName := "test_name"
	testValue := "test_value"
	metric := &io_prometheus_client.Metric{
		Label: []*io_prometheus_client.LabelPair{
			{Name: &testName, Value: &testValue},
		},
	}

	labelOne := "LABEL_ONE"
	labelOneVal := "value_one"
	labelTwo := "LABEL_TWO"
	labelTwoVal := "value_two"
	labelNames := []string{labelOne, labelTwo}
	labelValues := []string{labelOneVal, labelTwoVal}

	result := addServerlessLabels(metric, labelNames, labelValues)
	expected := &io_prometheus_client.Metric{
		Label: []*io_prometheus_client.LabelPair{
			{Name: &testName, Value: &testValue},
			{Name: &labelOne, Value: &labelOneVal},
			{Name: &labelTwo, Value: &labelTwoVal},
		},
	}
	assert.Equal(t, result.Label, expected.Label)
}

func TestGetHeaderTimeout(t *testing.T) {
	inputs := []string{"1.23", "100", "notvalid", "12.wrong"}
	errIsNil := []bool{true, true, false, false}

	for i, input := range inputs {
		_, err := getHeaderTimeout(input)
		if errIsNil[i] == true {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
		}
	}
}

func TestScrapeHeaders(t *testing.T) {
	metricExample := `# TYPE my_metric counter
	my_metric{} 0
	`
	timeoutHeader := "X-Prometheus-Scrape-Timeout-Seconds"
	tests := []struct {
		name            string
		headerVal       string
		expectNilCancel bool
	}{
		{
			name:      "timeout header parses",
			headerVal: "10",
		},
		{
			name:            "timeout header invalid",
			headerVal:       "invalid",
			expectNilCancel: true,
		},
	}

	zapLogger := logger.InitializeLogger()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			qp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := w.Write([]byte(metricExample))
				assert.NoError(t, err)
			}))
			defer qp.Close()

			url := getURL(strings.Split(qp.URL, ":")[2], "/metrics")

			req := &http.Request{
				Header: map[string][]string{timeoutHeader: {test.headerVal}},
			}
			queueProxy, queueProxyCancel, _, err := scrape(url, req.Header, zapLogger)
			assert.NoError(t, err)
			assert.NotNil(t, queueProxy)
			if test.expectNilCancel {
				assert.Nil(t, queueProxyCancel)
			} else {
				assert.NotNil(t, queueProxyCancel)
			}
		})
	}
}

func TestScrapeErr(t *testing.T) {
	metricExample := `# TYPE my_metric counter
	my_metric{} 0
	`
	zapLogger := logger.InitializeLogger()
	qp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(metricExample))
		assert.NoError(t, err)
	}))
	defer qp.Close()

	url := "not-a-real-url"

	req := &http.Request{}
	queueProxy, _, _, err := scrape(url, req.Header, zapLogger)
	assert.Error(t, err)
	assert.Nil(t, queueProxy)
}

func TestSanitizeMetrics(t *testing.T) {
	testCreated := "testing_created"
	testTotal := "testing_total"
	testNoConvert := "random_metric"
	untyped := io_prometheus_client.MetricType_UNTYPED
	counter := io_prometheus_client.MetricType_COUNTER
	gauge := io_prometheus_client.MetricType_GAUGE
	value := float64(4.5)

	tests := []struct {
		name     string
		input    *io_prometheus_client.MetricFamily
		expected *io_prometheus_client.MetricFamily
	}{
		{
			name: "test sanitize counter",
			input: &io_prometheus_client.MetricFamily{
				Name: &testCreated,
				Help: nil,
				Type: &untyped,
				Metric: []*io_prometheus_client.Metric{
					{
						Untyped: &io_prometheus_client.Untyped{
							Value: &value,
						},
					},
				},
			},
			expected: &io_prometheus_client.MetricFamily{
				Name: &testCreated,
				Help: nil,
				Type: &counter,
				Metric: []*io_prometheus_client.Metric{
					{
						Counter: &io_prometheus_client.Counter{
							Value: &value,
						},
					},
				},
			},
		},
		{
			name: "test sanitize gauge",
			input: &io_prometheus_client.MetricFamily{
				Name: &testTotal,
				Help: nil,
				Type: &untyped,
				Metric: []*io_prometheus_client.Metric{
					{
						Untyped: &io_prometheus_client.Untyped{
							Value: &value,
						},
					},
				},
			},
			expected: &io_prometheus_client.MetricFamily{
				Name: &testTotal,
				Help: nil,
				Type: &gauge,
				Metric: []*io_prometheus_client.Metric{
					{
						Gauge: &io_prometheus_client.Gauge{
							Value: &value,
						},
					},
				},
			},
		},
		{
			name: "test not able to convert",
			input: &io_prometheus_client.MetricFamily{
				Name: &testNoConvert,
				Help: nil,
				Type: &untyped,
				Metric: []*io_prometheus_client.Metric{
					{
						Untyped: &io_prometheus_client.Untyped{
							Value: &value,
						},
					},
				},
			},
			expected: nil,
		},
	}

	for _, test := range tests {
		actual := sanitizeMetrics(test.input)
		assert.True(t, reflect.DeepEqual(test.expected, actual))
	}
}

func TestAppMetrics(t *testing.T) {
	metricExample := `# HELP request_preprocess_seconds pre-process request latency
# TYPE request_preprocess_seconds histogram
request_preprocess_seconds_bucket{le="0.005",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="+Inf",model_name="custom-server-test"} 3.0
request_preprocess_seconds_sum{model_name="custom-server-test"} 0.00014145392924547195
request_preprocess_seconds_count{model_name="custom-server-test"} 3
`
	expected := `# HELP request_preprocess_seconds pre-process request latency
# TYPE request_preprocess_seconds histogram
request_preprocess_seconds_bucket{model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something",le="0.005"} 3
request_preprocess_seconds_bucket{model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something",le="+Inf"} 3
request_preprocess_seconds_sum{model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something"} 0.00014145392924547195
request_preprocess_seconds_count{model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something"} 3
`
	setEnvVars(t)
	tests := []struct {
		name             string
		queueproxy       string
		app              string
		output           string
		expectParseError bool
	}{
		{
			name:   "queueproxy metric only",
			app:    metricExample,
			output: expected,
		},
	}

	zapLogger := logger.InitializeLogger()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			qp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := w.Write([]byte(test.queueproxy))
				assert.NoError(t, err)
			}))
			defer qp.Close()

			app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := w.Write([]byte(test.app))
				assert.NoError(t, err)
			}))
			defer app.Close()

			psc := &ScrapeConfigurations{
				logger:         zapLogger,
				QueueProxyPort: strings.Split(qp.URL, ":")[2],
				AppPort:        strings.Split(app.URL, ":")[2],
			}
			req := &http.Request{}
			psc.handleStats(rec, req)
			assert.Equal(t, rec.Code, 200)
			assert.Contains(t, rec.Body.String(), test.output)

			parser := expfmt.TextParser{}
			mfMap, err := parser.TextToMetricFamilies(strings.NewReader(rec.Body.String()))
			if !test.expectParseError {
				assert.NoErrorf(t, err, "failed to parse metrics: %v", err)
			} else if err == nil && test.expectParseError {
				assert.False(t, test.expectParseError, "expected a prse error, got %+v", mfMap)
			}
		})
	}

}

func TestHandleStats(t *testing.T) {
	metricExample := `# TYPE my_metric counter
	my_metric{} 0
	`
	metricExampleWLabels := `# TYPE my_metric counter
my_metric{service_name="something",configuration_name="something",revision_name="something"} 0
`
	otherMetricExample := `# TYPE my_other_metric counter
my_other_metric{} 0
`
	histogramMetricExample := `# HELP request_preprocess_seconds pre-process request latency
# TYPE request_preprocess_seconds histogram
request_preprocess_seconds_bucket{le="0.005",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="+Inf",model_name="custom-server-test"} 3.0
request_preprocess_seconds_sum{model_name="custom-server-test"} 0.00014145392924547195
request_preprocess_seconds_count{model_name="custom-server-test"} 3
`
	histogramExpected := `# HELP request_preprocess_seconds pre-process request latency
# TYPE request_preprocess_seconds histogram
request_preprocess_seconds_bucket{model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something",le="0.005"} 3
request_preprocess_seconds_bucket{model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something",le="+Inf"} 3
request_preprocess_seconds_sum{model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something"} 0.00014145392924547195
request_preprocess_seconds_count{model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something"} 3
`
	setEnvVars(t)
	tests := []struct {
		name             string
		queueproxy       string
		app              string
		output           string
		expectParseError bool
	}{
		{
			name:       "queueproxy metric only",
			queueproxy: metricExample,
			output:     metricExample,
		},
		{
			name:   "app metric only",
			app:    metricExample,
			output: metricExampleWLabels,
		},
		{
			name:   "app metric histogram",
			app:    histogramMetricExample,
			output: histogramExpected,
		},
		{
			name:       "multiple metric",
			queueproxy: otherMetricExample,
			app:        metricExample,
			// since app metrics adds labels, the output should contain labels only for the app metrics
			output: otherMetricExample + metricExampleWLabels,
		},
		// when app and queueproxy share a metric, Prometheus will fail.
		{
			name:             "conflict metric",
			queueproxy:       metricExample + otherMetricExample,
			app:              metricExample,
			output:           metricExample + otherMetricExample + metricExampleWLabels,
			expectParseError: true,
		},
	}

	zapLogger := logger.InitializeLogger()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			qp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := w.Write([]byte(test.queueproxy))
				assert.NoError(t, err)
			}))
			defer qp.Close()

			app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := w.Write([]byte(test.app))
				assert.NoError(t, err)
			}))
			defer app.Close()

			psc := &ScrapeConfigurations{
				logger:         zapLogger,
				QueueProxyPort: strings.Split(qp.URL, ":")[2],
				AppPort:        strings.Split(app.URL, ":")[2],
			}
			req := &http.Request{}
			psc.handleStats(rec, req)
			assert.Equal(t, rec.Code, 200)
			assert.Contains(t, rec.Body.String(), test.output)

			parser := expfmt.TextParser{}
			mfMap, err := parser.TextToMetricFamilies(strings.NewReader(rec.Body.String()))
			if !test.expectParseError {
				assert.NoErrorf(t, err, "failed to parse metrics: %v", err)
			} else if err == nil && test.expectParseError {
				assert.False(t, test.expectParseError, "expected a prse error, got %+v", mfMap)
			}
		})
	}

}

func TestHandleStatsErr(t *testing.T) {
	zapLogger := logger.InitializeLogger()
	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fail.Close()
	pass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer pass.Close()
	failPort := strings.Split(fail.URL, ":")[2]
	passPort := strings.Split(pass.URL, ":")[2]

	tests := []struct {
		name       string
		queueproxy string
		app        string
	}{
		{"both pass", passPort, passPort},
		{"queue proxy pass", passPort, failPort},
		{"app pass", failPort, passPort},
		{"both fail", failPort, failPort},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sc := NewScrapeConfigs(zapLogger, test.queueproxy, test.app, DefaultQueueProxyMetricsPath)
			req := &http.Request{}
			rec := httptest.NewRecorder()
			sc.handleStats(rec, req)
			assert.Equal(t, 200, rec.Code)
		})
	}
}
