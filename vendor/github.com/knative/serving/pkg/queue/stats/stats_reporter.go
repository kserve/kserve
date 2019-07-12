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

package stats

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/knative/pkg/metrics"
	"github.com/knative/pkg/metrics/metricskey"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

const (
	requestCountN       = "request_count"
	responseTimeInMsecN = "request_latencies"
)

var (
	requestCountM = stats.Int64(
		requestCountN,
		"The number of requests that are routed to queue-proxy",
		stats.UnitDimensionless)
	responseTimeInMsecM = stats.Float64(
		responseTimeInMsecN,
		"The response time in millisecond",
		stats.UnitMilliseconds)
)

// StatsReporter defines the interface for sending queue-proxy metrics
type StatsReporter interface {
	ReportRequestCount(responseCode int, v int64) error
	ReportResponseTime(responseCode int, d time.Duration) error
}

// Reporter holds cached metric objects to report autoscaler metrics
type Reporter struct {
	initialized          bool
	ctx                  context.Context
	namespaceTagKey      tag.Key
	serviceTagKey        tag.Key
	configTagKey         tag.Key
	revisionTagKey       tag.Key
	responseCodeKey      tag.Key
	responseCodeClassKey tag.Key
}

// NewStatsReporter creates a reporter that collects and reports queue proxy metrics
func NewStatsReporter(ns, service, config, rev string) (*Reporter, error) {
	if ns == "" {
		return nil, errors.New("namespace must not be empty")
	}
	if config == "" {
		return nil, errors.New("config must not be empty")
	}
	if rev == "" {
		return nil, errors.New("revision must not be empty")
	}

	// Create the tag keys that will be used to add tags to our measurements.
	nsTag, err := tag.NewKey(metricskey.LabelNamespaceName)
	if err != nil {
		return nil, err
	}
	svcTag, err := tag.NewKey(metricskey.LabelServiceName)
	if err != nil {
		return nil, err
	}
	configTag, err := tag.NewKey(metricskey.LabelConfigurationName)
	if err != nil {
		return nil, err
	}
	revTag, err := tag.NewKey(metricskey.LabelRevisionName)
	if err != nil {
		return nil, err
	}
	responseCodeTag, err := tag.NewKey("response_code")
	if err != nil {
		return nil, err
	}
	responseCodeClassTag, err := tag.NewKey("response_code_class")
	if err != nil {
		return nil, err
	}

	// Create view to see our measurements.
	err = view.Register(
		&view.View{
			Description: "The number of requests that are routed to queue-proxy",
			Measure:     requestCountM,
			Aggregation: view.Sum(),
			TagKeys:     []tag.Key{nsTag, svcTag, configTag, revTag, responseCodeTag, responseCodeClassTag},
		},
		&view.View{
			Description: "The response time in millisecond",
			Measure:     responseTimeInMsecM,
			Aggregation: view.Distribution(1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 9000, 10000, 11000, 12000, 13000, 14000, 15000),
			TagKeys:     []tag.Key{nsTag, svcTag, configTag, revTag, responseCodeTag, responseCodeClassTag},
		},
	)
	if err != nil {
		return nil, err
	}

	// Note that service name can be an empty string, so it needs a special treatment.
	ctx, err := tag.New(
		context.Background(),
		tag.Insert(nsTag, ns),
		tag.Insert(svcTag, valueOrUnknown(service)),
		tag.Insert(configTag, config),
		tag.Insert(revTag, rev),
	)
	if err != nil {
		return nil, err
	}

	return &Reporter{
		initialized:          true,
		ctx:                  ctx,
		namespaceTagKey:      nsTag,
		serviceTagKey:        svcTag,
		configTagKey:         configTag,
		revisionTagKey:       revTag,
		responseCodeKey:      responseCodeTag,
		responseCodeClassKey: responseCodeClassTag,
	}, nil
}

func valueOrUnknown(v string) string {
	if v != "" {
		return v
	}
	return metricskey.ValueUnknown
}

// ReportRequestCount captures request count metric with value v.
func (r *Reporter) ReportRequestCount(responseCode int, v int64) error {
	if !r.initialized {
		return errors.New("StatsReporter is not initialized yet")
	}

	// Note that service names can be an empty string, so it needs a special treatment.
	ctx, err := tag.New(
		r.ctx,
		tag.Insert(r.responseCodeKey, strconv.Itoa(responseCode)),
		tag.Insert(r.responseCodeClassKey, responseCodeClass(responseCode)))
	if err != nil {
		return err
	}

	metrics.Record(ctx, requestCountM.M(v))
	return nil
}

// ReportResponseTime captures response time requests
func (r *Reporter) ReportResponseTime(responseCode int, d time.Duration) error {
	if !r.initialized {
		return errors.New("StatsReporter is not initialized yet")
	}

	// Note that service names can be an empty string, so it needs a special treatment.
	ctx, err := tag.New(
		r.ctx,
		tag.Insert(r.responseCodeKey, strconv.Itoa(responseCode)),
		tag.Insert(r.responseCodeClassKey, responseCodeClass(responseCode)))
	if err != nil {
		return err
	}

	// convert time.Duration in nanoseconds to milliseconds
	metrics.Record(ctx, responseTimeInMsecM.M(float64(d/time.Millisecond)))
	return nil
}

// responseCodeClass converts response code to a string of response code class.
// e.g. The response code class is "5xx" for response code 503.
func responseCodeClass(responseCode int) string {
	// Get the hundred digit of the response code and concatenate "xx".
	return strconv.Itoa(responseCode/100) + "xx"
}
