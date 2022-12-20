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
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var testEnvVarVal = "something"

func setEnvVars(t *testing.T) {
	for _, key := range EnvVars {
		t.Setenv(key, testEnvVarVal)
	}
}

var fullTest = `# HELP python_gc_objects_collected_total Objects collected during gc
# TYPE python_gc_objects_collected_total counter
python_gc_objects_collected_total{generation="0"} 12067.0
python_gc_objects_collected_total{generation="1"} 2146.0
python_gc_objects_collected_total{generation="2"} 14.0
# HELP python_gc_objects_uncollectable_total Uncollectable object found during GC
# TYPE python_gc_objects_uncollectable_total counter
python_gc_objects_uncollectable_total{generation="0"} 0.0
python_gc_objects_uncollectable_total{generation="1"} 0.0
python_gc_objects_uncollectable_total{generation="2"} 0.0
# HELP python_gc_collections_total Number of times this generation was collected
# TYPE python_gc_collections_total counter
python_gc_collections_total{generation="0"} 443.0
python_gc_collections_total{generation="1"} 40.0
python_gc_collections_total{generation="2"} 3.0
# HELP python_info Python platform information
# TYPE python_info gauge
python_info{implementation="CPython",major="3",minor="9",patchlevel="16",version="3.9.16"} 1.0
# HELP process_virtual_memory_bytes Virtual memory size in bytes.
# TYPE process_virtual_memory_bytes gauge
process_virtual_memory_bytes 2.641526784e+09
# HELP process_resident_memory_bytes Resident memory size in bytes.
# TYPE process_resident_memory_bytes gauge
process_resident_memory_bytes 1.55590656e+08
# HELP process_start_time_seconds Start time of the process since unix epoch in seconds.
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1.67106640931e+09
# HELP process_cpu_seconds_total Total user and system CPU time spent in seconds.
# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 5.35
# HELP process_open_fds Number of open file descriptors.
# TYPE process_open_fds gauge
process_open_fds 16.0
# HELP process_max_fds Maximum number of open file descriptors.
# TYPE process_max_fds gauge
process_max_fds 1.048576e+06
# HELP request_preprocess_seconds pre-process request latency
# TYPE request_preprocess_seconds histogram
request_preprocess_seconds_bucket{le="0.005",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="0.01",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="0.025",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="0.05",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="0.075",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="0.1",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="0.25",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="0.5",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="0.75",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="1.0",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="2.5",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="5.0",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="7.5",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="10.0",model_name="custom-server-test"} 3.0
request_preprocess_seconds_bucket{le="+Inf",model_name="custom-server-test"} 3.0
request_preprocess_seconds_count{model_name="custom-server-test"} 3.0
request_preprocess_seconds_sum{model_name="custom-server-test"} 0.00016299448907375336
# HELP request_preprocess_seconds_created pre-process request latency
# TYPE request_preprocess_seconds_created gauge
request_preprocess_seconds_created{model_name="custom-server-test"} 1.6710664789782867e+09
# HELP request_postprocess_seconds post-process request latency
# TYPE request_postprocess_seconds histogram
request_postprocess_seconds_bucket{le="0.005",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="0.01",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="0.025",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="0.05",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="0.075",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="0.1",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="0.25",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="0.5",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="0.75",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="1.0",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="2.5",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="5.0",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="7.5",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="10.0",model_name="custom-server-test"} 3.0
request_postprocess_seconds_bucket{le="+Inf",model_name="custom-server-test"} 3.0
request_postprocess_seconds_count{model_name="custom-server-test"} 3.0
request_postprocess_seconds_sum{model_name="custom-server-test"} 6.933044642210007e-05
# HELP request_postprocess_seconds_created post-process request latency
# TYPE request_postprocess_seconds_created gauge
request_postprocess_seconds_created{model_name="custom-server-test"} 1.6710664809811184e+09
# HELP request_predict_seconds predict request latency
# TYPE request_predict_seconds histogram
request_predict_seconds_bucket{le="0.005",model_name="custom-server-test"} 0.0
request_predict_seconds_bucket{le="0.01",model_name="custom-server-test"} 0.0
request_predict_seconds_bucket{le="0.025",model_name="custom-server-test"} 0.0
request_predict_seconds_bucket{le="0.05",model_name="custom-server-test"} 0.0
request_predict_seconds_bucket{le="0.075",model_name="custom-server-test"} 0.0
request_predict_seconds_bucket{le="0.1",model_name="custom-server-test"} 0.0
request_predict_seconds_bucket{le="0.25",model_name="custom-server-test"} 0.0
request_predict_seconds_bucket{le="0.5",model_name="custom-server-test"} 0.0
request_predict_seconds_bucket{le="0.75",model_name="custom-server-test"} 0.0
request_predict_seconds_bucket{le="1.0",model_name="custom-server-test"} 0.0
request_predict_seconds_bucket{le="2.5",model_name="custom-server-test"} 3.0
request_predict_seconds_bucket{le="5.0",model_name="custom-server-test"} 3.0
request_predict_seconds_bucket{le="7.5",model_name="custom-server-test"} 3.0
request_predict_seconds_bucket{le="10.0",model_name="custom-server-test"} 3.0
request_predict_seconds_bucket{le="+Inf",model_name="custom-server-test"} 3.0
request_predict_seconds_count{model_name="custom-server-test"} 3.0
request_predict_seconds_sum{model_name="custom-server-test"} 6.006712835282087
# HELP request_predict_seconds_created predict request latency
# TYPE request_predict_seconds_created gauge
request_predict_seconds_created{model_name="custom-server-test"} 1.6710664789786139e+09
`

//
//var fullTestTwo = `# HELP ts_inference_requests_total Total number of inference requests.
//# TYPE ts_inference_requests_total counter
//ts_inference_requests_total{uuid="628584d1-84de-4997-9429-cf9330e52725",model_name="text-classification",model_version="default",} 7.0
//# HELP ts_queue_latency_microseconds Cumulative queue duration in microseconds
//# TYPE ts_queue_latency_microseconds counter
//# HELP ts_inference_latency_microseconds Cumulative inference duration in microseconds
//# TYPE ts_inference_latency_microseconds counter`

//# HELP request_explain_seconds explain request latency
//# TYPE request_explain_seconds histogram`
//
//func TestGetServerlessLabelVals(t *testing.T) {
//	setEnvVars(t)
//	labelVals := getServerlessLabelVals()
//	for idx, val := range labelVals {
//		assert.Equal(t, os.Getenv(EnvVars[idx]), val)
//	}
//}
//
//func TestAddServerlessLabels(t *testing.T) {
//	testName := "test_name"
//	testValue := "test_value"
//	metric := &io_prometheus_client.Metric{
//		Label: []*io_prometheus_client.LabelPair{
//			{Name: &testName, Value: &testValue},
//		},
//	}
//
//	labelOne := "LABEL_ONE"
//	labelOneVal := "value_one"
//	labelTwo := "LABEL_TWO"
//	labelTwoVal := "value_two"
//	labelNames := []string{labelOne, labelTwo}
//	labelValues := []string{labelOneVal, labelTwoVal}
//
//	result := addServerlessLabels(metric, labelNames, labelValues)
//	expected := &io_prometheus_client.Metric{
//		Label: []*io_prometheus_client.LabelPair{
//			{Name: &testName, Value: &testValue},
//			{Name: &labelOne, Value: &labelOneVal},
//			{Name: &labelTwo, Value: &labelTwoVal},
//		},
//	}
//	assert.Equal(t, result.Label, expected.Label)
//}
//
//func TestGetHeaderTimeout(t *testing.T) {
//	inputs := []string{"1.23", "100", "notvalid", "12.wrong"}
//	errIsNil := []bool{true, true, false, false}
//
//	for i, input := range inputs {
//		_, err := getHeaderTimeout(input)
//		if errIsNil[i] == true {
//			assert.NoError(t, err)
//		} else {
//			assert.Error(t, err)
//		}
//	}
//}
//
//func TestNegotiateMetricsFromat(t *testing.T) {
//	contentTypes := []string{"", "random", "text/plain;version=0.0.4;q=0.5,*/*;q=0.1", `application/openmetrics-text; version=1.0.0; charset=utf-8`}
//	expected := []expfmt.Format{expfmt.FmtText, expfmt.FmtText, expfmt.FmtText, expfmt.FmtOpenMetrics}
//
//	for i, contentType := range contentTypes {
//		result := negotiateMetricsFormat(contentType)
//		assert.Equal(t, expected[i], result)
//	}
//}
//
//func TestScrapeHeaders(t *testing.T) {
//	metricExample := `# TYPE my_metric counter
//	my_metric{} 0
//	`
//	timeoutHeader := "X-Prometheus-Scrape-Timeout-Seconds"
//	tests := []struct {
//		name            string
//		headerVal       string
//		expectNilCancel bool
//	}{
//		{
//			name:      "timeout header parses",
//			headerVal: "10",
//		},
//		{
//			name:            "timeout header invalid",
//			headerVal:       "invalid",
//			expectNilCancel: true,
//		},
//	}
//
//	zapLogger := logger.InitializeLogger()
//	for _, test := range tests {
//		t.Run(test.name, func(t *testing.T) {
//			promRegistry = prometheus.NewRegistry()
//			qp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//				_, err := w.Write([]byte(metricExample))
//				assert.NoError(t, err)
//			}))
//			defer qp.Close()
//
//			url := getURL(strings.Split(qp.URL, ":")[2], "/metrics")
//
//			req := &http.Request{
//				Header: map[string][]string{timeoutHeader: {test.headerVal}},
//			}
//			queueProxy, queueProxyCancel, _, err := scrape(url, req.Header, zapLogger)
//			assert.NoError(t, err)
//			assert.NotNil(t, queueProxy)
//			if test.expectNilCancel {
//				assert.Nil(t, queueProxyCancel)
//			} else {
//				assert.NotNil(t, queueProxyCancel)
//			}
//		})
//	}
//}
//
//func TestScrapeErr(t *testing.T) {
//	metricExample := `# TYPE my_metric counter
//	my_metric{} 0
//	`
//	zapLogger := logger.InitializeLogger()
//	promRegistry = prometheus.NewRegistry()
//	qp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		_, err := w.Write([]byte(metricExample))
//		assert.NoError(t, err)
//	}))
//	defer qp.Close()
//
//	url := "not-a-real-url"
//
//	req := &http.Request{}
//	queueProxy, _, _, err := scrape(url, req.Header, zapLogger)
//	assert.Error(t, err)
//	assert.Nil(t, queueProxy)
//}

func TestAppMetrics(t *testing.T) {
	metricExample := `# HELP request_preprocess_seconds_created pre-process request latency
# TYPE request_preprocess_seconds_created counter
request_preprocess_seconds_bucket_created{model_name="custom-server-test"} 3.0
`
	//	metricExample := `# HELP request_preprocess_seconds pre-process request latency
	//# TYPE request_preprocess_seconds histogram
	//request_preprocess_seconds_bucket{le="0.005",model_name="custom-server-test"} 3.0
	//request_preprocess_seconds_bucket{le="0.01",model_name="custom-server-test"} 3.0
	//request_preprocess_seconds_count{model_name="custom-server-test"} 3.0
	//request_preprocess_seconds_sum{model_name="custom-server-test"} 0.00014145392924547195
	//`
	expected := `# HELP request_preprocess_seconds pre-process request latency
# TYPE request_preprocess_seconds histogram
request_preprocess_seconds_bucket{le="0.005",model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something"} 3
request_preprocess_seconds_bucket{le="0.01",model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something"} 3
request_preprocess_seconds_sum{model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something"} 0.00014145392924547195
request_preprocess_seconds_count{model_name="custom-server-test",service_name="something",configuration_name="something",revision_name="something"} 3
`
	setEnvVars(t)
	_ = metricExample
	_ = expected
	//_ = fullTest
	//_ = fullTestTwo
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
			output: metricExample,
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

//
//func TestHandleStats(t *testing.T) {
//	metricExample := `# TYPE my_metric counter
//	my_metric{} 0
//	`
//	metricExampleWLabels := `# TYPE my_metric counter
//my_metric{service_name="something",configuration_name="something",revision_name="something"} 0
//`
//	otherMetricExample := `# TYPE my_other_metric counter
//my_other_metric{} 0
//`
//	setEnvVars(t)
//	tests := []struct {
//		name             string
//		queueproxy       string
//		app              string
//		output           string
//		expectParseError bool
//	}{
//		{
//			name:       "queueproxy metric only",
//			queueproxy: metricExample,
//			output:     metricExample,
//		},
//		{
//			name:   "app metric only",
//			app:    metricExample,
//			output: metricExampleWLabels,
//		},
//		{
//			name:       "multiple metric",
//			queueproxy: otherMetricExample,
//			app:        metricExample,
//			// since app metrics adds labels, the output should contain labels only for the app metrics
//			output: otherMetricExample + metricExampleWLabels,
//		},
//		// when app and queueproxy share a metric, Prometheus will fail.
//		{
//			name:             "conflict metric",
//			queueproxy:       metricExample + otherMetricExample,
//			app:              metricExample,
//			output:           metricExample + otherMetricExample + metricExampleWLabels,
//			expectParseError: true,
//		},
//	}
//
//	zapLogger := logger.InitializeLogger()
//	for _, test := range tests {
//		t.Run(test.name, func(t *testing.T) {
//			promRegistry = prometheus.NewRegistry()
//			rec := httptest.NewRecorder()
//			qp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//				_, err := w.Write([]byte(test.queueproxy))
//				assert.NoError(t, err)
//			}))
//			defer qp.Close()
//
//			app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//				_, err := w.Write([]byte(test.app))
//				assert.NoError(t, err)
//			}))
//			defer app.Close()
//
//			psc := &ScrapeConfigurations{
//				logger:         zapLogger,
//				QueueProxyPort: strings.Split(qp.URL, ":")[2],
//				AppPort:        strings.Split(app.URL, ":")[2],
//			}
//			req := &http.Request{}
//			psc.handleStats(rec, req)
//			assert.Equal(t, rec.Code, 200)
//			assert.Contains(t, rec.Body.String(), test.output)
//
//			parser := expfmt.TextParser{}
//			mfMap, err := parser.TextToMetricFamilies(strings.NewReader(rec.Body.String()))
//			if !test.expectParseError {
//				assert.NoErrorf(t, err, "failed to parse metrics: %v", err)
//			} else if err == nil && test.expectParseError {
//				assert.False(t, test.expectParseError, "expected a prse error, got %+v", mfMap)
//			}
//		})
//	}
//
//}
//
//func TestHandleStatsErr(t *testing.T) {
//	zapLogger := logger.InitializeLogger()
//	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		w.WriteHeader(http.StatusInternalServerError)
//	}))
//	defer fail.Close()
//	pass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		w.WriteHeader(http.StatusOK)
//	}))
//	defer pass.Close()
//	failPort := strings.Split(fail.URL, ":")[2]
//	passPort := strings.Split(pass.URL, ":")[2]
//
//	tests := []struct {
//		name       string
//		queueproxy string
//		app        string
//	}{
//		{"both pass", passPort, passPort},
//		{"queue proxy pass", passPort, failPort},
//		{"app pass", failPort, passPort},
//		{"both fail", failPort, failPort},
//	}
//	for _, test := range tests {
//		t.Run(test.name, func(t *testing.T) {
//			sc := NewScrapeConfigs(zapLogger, test.queueproxy, test.app, DefaultQueueProxyMetricsPath)
//			req := &http.Request{}
//			rec := httptest.NewRecorder()
//			sc.handleStats(rec, req)
//			assert.Equal(t, 200, rec.Code)
//		})
//	}
//}
