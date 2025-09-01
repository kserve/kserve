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
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

import "knative.dev/serving/pkg/queue/sharedmain"

var (
	EnvVars   = []string{"SERVING_SERVICE", "SERVING_CONFIGURATION", "SERVING_REVISION"}
	LabelKeys = []string{"service_name", "configuration_name", "revision_name"}
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

// sanitizeMetrics attempts to convert UNTYPED metrics into either a gauge or counter.
// counter metric names with _created and gauge metric names with _total are converted due to irregularities
// observed in the conversion of these metrics from text to metric families.
func sanitizeMetrics(mf *io_prometheus_client.MetricFamily) *io_prometheus_client.MetricFamily {
	if strings.HasSuffix(*mf.Name, "_created") {
		counter := io_prometheus_client.MetricType_COUNTER
		var newMetric []*io_prometheus_client.Metric
		for _, metric := range mf.Metric {
			newMetric = append(newMetric, &io_prometheus_client.Metric{
				Label: metric.Label,
				Counter: &io_prometheus_client.Counter{
					Value: metric.Untyped.Value,
				},
				TimestampMs: metric.TimestampMs,
			})
		}
		return &io_prometheus_client.MetricFamily{
			Name:   mf.Name,
			Help:   mf.Help,
			Type:   &counter,
			Metric: newMetric,
		}
	}

	if strings.HasSuffix(*mf.Name, "_total") {
		gauge := io_prometheus_client.MetricType_GAUGE
		var newMetric []*io_prometheus_client.Metric
		for _, metric := range mf.Metric {
			newMetric = append(newMetric, &io_prometheus_client.Metric{
				Label: metric.Label,
				Gauge: &io_prometheus_client.Gauge{
					Value: metric.Untyped.Value,
				},
				TimestampMs: metric.TimestampMs,
			})
		}
		return &io_prometheus_client.MetricFamily{
			Name:   mf.Name,
			Help:   mf.Help,
			Type:   &gauge,
			Metric: newMetric,
		}
	}
	return nil
}

func scrapeAndWriteAppMetrics(mfs map[string]*io_prometheus_client.MetricFamily, w io.Writer, format expfmt.Format, logger *zap.Logger) error {
	var errs error
	labelValues := getServerlessLabelVals()

	for _, metricFamily := range mfs {
		var newMetric []*io_prometheus_client.Metric
		var mf *io_prometheus_client.MetricFamily

		// Some metrics from kserve-container are UNTYPED. This can cause errors in the promtheus scraper.
		// These metrics seem to be either gauges or counters. For now, avoid these errors by sanitizing the metrics
		// based on the metric name. If the metric can't be converted, we log an error. In the future, we should
		// figure out the root cause of this. (Possibly due to open metrics being read in as text and converted to MetricFamily)
		if *metricFamily.Type == io_prometheus_client.MetricType_UNTYPED {
			mf = sanitizeMetrics(metricFamily)
			if mf == nil {
				// if the metric fails to convert, discard it and keep exporting the rest of the metrics
				logger.Error("failed to parse untyped metric", zap.Any("metric name", metricFamily.Name))
				continue
			}
		} else {
			mf = metricFamily
		}

		// create a new list of Metric with the added serverless labels to each individual Metric
		for _, metric := range mf.Metric {
			m := addServerlessLabels(metric, LabelKeys, labelValues)
			newMetric = append(newMetric, m)
		}
		mf.Metric = newMetric

		_, err := expfmt.MetricFamilyToText(w, mf)
		if err != nil {
			logger.Error("multierr", zap.Error(err))
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
		if err := resp.Body.Close(); err != nil {
			cancel()
			return nil, nil, "", err
		}
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
		if queueProxy != nil {
			err = queueProxy.Close()
			if err != nil {
				sc.logger.Error("queue proxy connection is not closed", zap.Error(err))
			}
		}
		if application != nil {
			err = application.Close()
			if err != nil {
				sc.logger.Error("application connection is not closed", zap.Error(err))
			}
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

	// Scrape app metrics if defined
	if sc.AppPort != "" {
		kserveContainerURL := getURL(sc.AppPort, sc.AppPath)
		var contentType string
		if application, appCancel, contentType, err = scrape(kserveContainerURL, r.Header, sc.logger); err != nil {
			sc.logger.Error("failed scraping application metrics", zap.Error(err), zap.String("content type", contentType))
		}
	}

	// Since we convert the scraped metrics to text, set the format as text even if
	// the content type is originally open metrics.
	format := expfmt.FmtText
	w.Header().Set("Content-Type", string(format))

	if queueProxy != nil {
		_, err = io.Copy(w, queueProxy)
		if err != nil {
			sc.logger.Error("failed to scraping and writing queue proxy metrics", zap.Error(err))
		}
	}

	if application != nil {
		var err error
		var parser expfmt.TextParser
		var mfs map[string]*io_prometheus_client.MetricFamily
		mfs, err = parser.TextToMetricFamilies(application)
		if err != nil {
			sc.logger.Error("error converting text to metric families", zap.Error(err), zap.Any("metric families return value", mfs))
		}
		if err = scrapeAndWriteAppMetrics(mfs, w, format, sc.logger); err != nil {
			sc.logger.Error("failed scraping and writing metrics", zap.Error(err))
		}
	}
}

func main() {
	zapLogger := logger.InitializeLogger()
	mux := http.NewServeMux()
	ctx, cancel := context.WithCancel(context.Background())
	aggregateMetricsPort := os.Getenv(QueueProxyAggregatePrometheusMetricsPortEnvVarKey)
	sc := NewScrapeConfigs(
		zapLogger,
		QueueProxyMetricsPort,
		os.Getenv(KServeContainerPrometheusMetricsPortEnvVarKey),
		os.Getenv(KServeContainerPrometheusMetricsPathEnvVarKey),
	)
	mux.HandleFunc(`/metrics`, sc.handleStats)
	l, err := net.Listen("tcp", fmt.Sprintf(":%v", aggregateMetricsPort))
	if err != nil {
		zapLogger.Error("error listening on status port", zap.Error(err))
		return
	}

	errCh := make(chan error)
	go func() {
		zapLogger.Info(fmt.Sprintf("Starting stats server on port %v", aggregateMetricsPort))
		if err = http.Serve(l, mux); err != nil {
			errCh <- fmt.Errorf("stats server failed to serve: %w", err)
		}
	}()

	go func() {
		if err := sharedmain.Main(); err != nil {
			errCh <- err
		}
		// sharedMain exited without error which means graceful shutdown due to SIGTERM / SIGINT signal
		// Attempt a graceful shutdown of stats server
		cancel()
	}()

	// Blocks until sharedMain or server exits unexpectedly or SIGTERM / SIGINT signal is received.
	select {
	case err := <-errCh:
		zapLogger.Error("error serving aggregate metrics", zap.Error(err))
		os.Exit(1)

	case <-ctx.Done():
		zapLogger.Info("Attempting graceful shutdown of stats server")
		err := l.Close()
		if err != nil {
			zapLogger.Error("failed to shutdown stats server", zap.Error(err))
			os.Exit(1)
		}
	}
	zapLogger.Info("Stats server has successfully terminated")
}
