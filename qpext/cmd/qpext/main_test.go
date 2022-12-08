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
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestNegotiateMetricsFromat(t *testing.T) {
	contentTypes := []string{"", "random", "text/plain;version=0.0.4;q=0.5,*/*;q=0.1", `application/openmetrics-text; version=1.0.0; charset=utf-8`}
	expected := []expfmt.Format{expfmt.FmtText, expfmt.FmtText, expfmt.FmtText, expfmt.FmtOpenMetrics}

	for i, contentType := range contentTypes {
		result := negotiateMetricsFormat(contentType)
		assert.Equal(t, expected[i], result)
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
			promRegistry = prometheus.NewRegistry()
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
	promRegistry = prometheus.NewRegistry()
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
			promRegistry = prometheus.NewRegistry()
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
