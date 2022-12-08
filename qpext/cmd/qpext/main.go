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
	"context"
	"fmt"
	"github.com/hashicorp/go-multierror"
	logger "github.com/kserve/kserve/qpext"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"go.uber.org/zap"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

import "knative.dev/serving/pkg/queue/sharedmain"

var (
	promRegistry *prometheus.Registry
	EnvVars      = []string{"SERVING_SERVICE", "SERVING_CONFIGURATION", "SERVING_REVISION"}
	LabelKeys    = []string{"service_name", "configuration_name", "revision_name"}
)

const (
	// aggregate scraping env vars from kserve/pkg/constants
	KServeContainerPrometheusMetricsPortEnvVarKey     = "KSERVE_CONTAINER_PROMETHEUS_METRICS_PORT"
	KServeContainerPrometheusMetricsPathEnvVarKey     = "KSERVE_CONTAINER_PROMETHEUS_METRICS_PATH"
	QueueProxyAggregatePrometheusMetricsPortEnvVarKey = "AGGREGATE_PROMETHEUS_METRICS_PORT"
	QueueProxyMetricsPort                             = "9091"
	DefaultQueueProxyMetricsPath                      = "/metrics"
	prometheusTimeoutHeader                           = "X-Prometheus-Scrape-Timeout-Seconds"
)

type ScrapeConfigurations struct {
	logger         *zap.Logger
	QueueProxyPath string `json:"path"`
	QueueProxyPort string `json:"port"`
	AppPort        string
	AppPath        string
}

func getURL(port string, path string) string {
	return fmt.Sprintf("http://localhost:%s%s", port, path)
}

// getHeaderTimeout parse a string like (1.234) representing number of seconds
func getHeaderTimeout(timeout string) (time.Duration, error) {
	timeoutSeconds, err := strconv.ParseFloat(timeout, 64)
	if err != nil {
		return 0 * time.Second, err
	}

	return time.Duration(timeoutSeconds * 1e9), nil
}

func applyHeaders(into http.Header, from http.Header, keys ...string) {
	for _, key := range keys {
		val := from.Get(key)
		if val != "" {
			into.Set(key, val)
		}
	}
}

func negotiateMetricsFormat(contentType string) expfmt.Format {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && mediaType == expfmt.OpenMetricsType {
		return expfmt.FmtOpenMetrics
	}
	return expfmt.FmtText
}

// scrapeAndWriteAgentMetrics gathers a slice of prometheus metric families and encodes the metrics.
func scrapeAndWriteAgentMetrics(w io.Writer) error {
	mfs, err := promRegistry.Gather()
	if err != nil {
		return err
	}
	enc := expfmt.NewEncoder(w, expfmt.FmtText)
	var errs error
	for _, mf := range mfs {
		if err = enc.Encode(mf); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

func getServerlessLabelVals() []string {
	var labelValues []string
	for _, envVar := range EnvVars {
		labelValues = append(labelValues, os.Getenv(envVar))
	}
	return labelValues
}

// addServerlessLabels adds the serverless labels to the prometheus metrics that are imported in from the application.
// this is done so that the prometheus metrics (both queue-proxy's and kserve-container's) can be easily queried together.
func addServerlessLabels(metric *io_prometheus_client.Metric, labelKeys []string, labelValues []string) *io_prometheus_client.Metric {
	// LabelKeys, EnvVars, and LabelVals are []string to enforce setting them in order (helps with testing)
	for idx, name := range labelKeys {
		labelName := name
		labelValue := labelValues[idx]
		newLabelPair := &io_prometheus_client.LabelPair{
			Name:  &labelName,
			Value: &labelValue,
		}
		metric.Label = append(metric.Label, newLabelPair)
	}
	return metric
}

func scrapeAndWriteMetrics(mfs map[string]*io_prometheus_client.MetricFamily, w io.Writer, logger *zap.Logger) error {
	enc := expfmt.NewEncoder(w, expfmt.FmtText)
	var errs error
	labelValues := getServerlessLabelVals()
	for _, mf := range mfs {
		var newMetric []*io_prometheus_client.Metric
		// create a new list of Metric with the added serverless labels to each individual Metric
		for _, metric := range mf.Metric {
			m := addServerlessLabels(metric, LabelKeys, labelValues)
			newMetric = append(newMetric, m)
		}
		mf.Metric = newMetric

		if err := enc.Encode(mf); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

// scrape sends a request to the provided url to scrape metrics from
// This will attempt to mimic some of Prometheus functionality by passing some headers through
// scrape returns the scraped metrics reader as well as the response's "Content-Type" header to determine the metrics format
func scrape(url string, header http.Header, logger *zap.Logger) (io.ReadCloser, context.CancelFunc, string, error) {
	var cancel context.CancelFunc
	ctx := context.Background()
	if timeoutString := header.Get(prometheusTimeoutHeader); timeoutString != "" {
		timeout, err := getHeaderTimeout(timeoutString)
		if err != nil {
			logger.Error("Failed to parse timeout header", zap.Error(err), zap.String("timeout", timeoutString))
		} else {
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, cancel, "", err
	}

	applyHeaders(req.Header, header, "Accept",
		"User-Agent",
		prometheusTimeoutHeader,
	)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, cancel, "", fmt.Errorf("error scraping %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, cancel, "", fmt.Errorf("error scraping %s, status code: %v", url, resp.StatusCode)
	}
	format := resp.Header.Get("Content-Type")
	return resp.Body, cancel, format, nil
}

func NewScrapeConfigs(logger *zap.Logger, queueProxyPort string, appPort string, appPath string) *ScrapeConfigurations {
	return &ScrapeConfigurations{
		logger:         logger,
		QueueProxyPath: DefaultQueueProxyMetricsPath,
		QueueProxyPort: queueProxyPort,
		AppPort:        appPort,
		AppPath:        appPath,
	}
}

func (sc *ScrapeConfigurations) handleStats(w http.ResponseWriter, r *http.Request) {
	var err error
	var queueProxy, application io.ReadCloser
	var queueProxyCancel, appCancel context.CancelFunc

	defer func() {
		if application != nil {
			application.Close()
		}
		if queueProxyCancel != nil {
			queueProxyCancel()
		}
		if appCancel != nil {
			appCancel()
		}
	}()

	// Gather all the metrics we will merge
	if sc.QueueProxyPort != "" {
		queueProxyURL := getURL(sc.QueueProxyPort, sc.QueueProxyPath)
		if queueProxy, queueProxyCancel, _, err = scrape(queueProxyURL, r.Header, sc.logger); err != nil {
			sc.logger.Error("failed scraping queue proxy metrics", zap.Error(err))
		}
	}

	// Scrape app metrics if defined and capture their format
	var format expfmt.Format
	if sc.AppPort != "" {
		kserveContainerURL := getURL(sc.AppPort, sc.AppPath)
		var contentType string
		if application, appCancel, contentType, err = scrape(kserveContainerURL, r.Header, sc.logger); err != nil {
			sc.logger.Error("failed scraping application metrics", zap.Error(err))
		}
		format = negotiateMetricsFormat(contentType)
	} else {
		// Without app metrics format use a default
		format = expfmt.FmtText
	}

	w.Header().Set("Content-Type", string(format))

	// Write out the metrics
	//TODO: do we need this?
	if err = scrapeAndWriteAgentMetrics(io.Writer(w)); err != nil {
		sc.logger.Error("failed scraping and writing agent metrics", zap.Error(err))
	}

	if queueProxy != nil {
		_, err = io.Copy(w, queueProxy)
		if err != nil {
			sc.logger.Error("failed to scraping and writing queue proxy metrics", zap.Error(err))
		}
	}

	// App metrics must go last because if they are FmtOpenMetrics,
	// they will have a trailing "# EOF" which terminates the full exposition
	if application != nil {
		var parser expfmt.TextParser
		var mfs map[string]*io_prometheus_client.MetricFamily
		mfs, err = parser.TextToMetricFamilies(application)
		if err != nil {
			sc.logger.Error("error text to metric families", zap.Error(err))
		}
		if err = scrapeAndWriteMetrics(mfs, w, sc.logger); err != nil {
			sc.logger.Error("failed scraping and writing metrics", zap.Error(err))
		}
	}
}

func main() {
	zapLogger := logger.InitializeLogger()
	promRegistry = prometheus.NewRegistry()
	mux := http.NewServeMux()
	ctx := context.Background()
	sc := NewScrapeConfigs(
		zapLogger,
		QueueProxyMetricsPort,
		os.Getenv(KServeContainerPrometheusMetricsPortEnvVarKey),
		os.Getenv(KServeContainerPrometheusMetricsPathEnvVarKey),
	)
	mux.HandleFunc(`/metrics`, sc.handleStats)
	l, err := net.Listen("tcp", fmt.Sprintf(":%v", os.Getenv(QueueProxyAggregatePrometheusMetricsPortEnvVarKey)))
	if err != nil {
		zapLogger.Error("error listening on status port", zap.Error(err))
		return
	}

	defer l.Close()

	go func() {
		if err = http.Serve(l, mux); err != nil {
			zapLogger.Error("error serving aggregate metrics", zap.Error(err))
			fmt.Println(err)
			select {
			case <-ctx.Done():
				// We are shutting down already, don't trigger SIGTERM
				return
			default:
			}
		}
	}()
	zapLogger.Info("Stats server has successfully started")
	if sharedmain.Main() != nil {
		os.Exit(1)
	}
	// Wait for the agent to be shut down.
	<-ctx.Done()
	zapLogger.Info("Stats server has successfully terminated")
}
