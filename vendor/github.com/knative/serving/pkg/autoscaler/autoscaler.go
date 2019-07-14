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

package autoscaler

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/knative/pkg/logging"
	"github.com/knative/serving/pkg/resources"
	"go.uber.org/zap"
	corev1informers "k8s.io/client-go/informers/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

const (
	// bucketSize is the size of the buckets of stats we create.
	bucketSize time.Duration = 2 * time.Second
)

// Stat defines a single measurement at a point in time
type Stat struct {
	// The time the data point was received by autoscaler.
	Time *time.Time

	// The unique identity of this pod.  Used to count how many pods
	// are contributing to the metrics.
	PodName string

	// Average number of requests currently being handled by this pod.
	AverageConcurrentRequests float64

	// Part of AverageConcurrentRequests, for requests going through a proxy.
	AverageProxiedConcurrentRequests float64

	// Number of requests received since last Stat (approximately QPS).
	RequestCount int32

	// Part of RequestCount, for requests going through a proxy.
	ProxiedRequestCount int32
}

// StatMessage wraps a Stat with identifying information so it can be routed
// to the correct receiver.
type StatMessage struct {
	Key  string
	Stat Stat
}

// statsBucket keeps all the stats that fall into a defined bucket.
type statsBucket map[string][]*Stat

// add adds a Stat to the bucket. Stats from the same pod will be
// collapsed.
func (b statsBucket) add(stat *Stat) {
	b[stat.PodName] = append(b[stat.PodName], stat)
}

// concurrency calculates the overall concurrency as measured by this
// bucket. All stats that belong to the same pod will be averaged.
// The overall concurrency is the sum of the measured concurrency of all
// pods (including activator metrics).
func (b statsBucket) concurrency() float64 {
	var total float64
	for _, podStats := range b {
		var subtotal float64
		for _, stat := range podStats {
			// Proxied requests have been counted at the activator. Subtract
			// AverageProxiedConcurrentRequests to avoid double counting.
			subtotal += stat.AverageConcurrentRequests - stat.AverageProxiedConcurrentRequests
		}
		total += subtotal / float64(len(podStats))
	}

	return total
}

// Autoscaler stores current state of an instance of an autoscaler
type Autoscaler struct {
	*DynamicConfig
	namespace       string
	revisionService string
	endpointsLister corev1listers.EndpointsLister
	reporter        StatsReporter

	// State in panic mode. Carries over multiple Scale calls.
	panicTime    *time.Time
	maxPanicPods int32

	// targetMutex guards the elements in the block below.
	targetMutex sync.RWMutex
	target      float64

	// statsMutex guards the elements in the block below.
	statsMutex sync.Mutex
	bucketed   map[time.Time]statsBucket
}

// New creates a new instance of autoscaler
func New(
	dynamicConfig *DynamicConfig,
	namespace string,
	revisionService string,
	endpointsInformer corev1informers.EndpointsInformer,
	target float64,
	reporter StatsReporter) (*Autoscaler, error) {
	if endpointsInformer == nil {
		return nil, errors.New("'endpointsEnformer' must not be nil")
	}
	if reporter == nil {
		return nil, errors.New("stats reporter must not be nil")
	}

	// A new instance of autoscaler is created without panic mode.
	reporter.ReportPanic(0)

	return &Autoscaler{
		DynamicConfig:   dynamicConfig,
		namespace:       namespace,
		revisionService: revisionService,
		endpointsLister: endpointsInformer.Lister(),
		target:          target,
		bucketed:        make(map[time.Time]statsBucket),
		reporter:        reporter,
	}, nil
}

// Update reconfigures the UniScaler according to the DeciderSpec.
func (a *Autoscaler) Update(spec DeciderSpec) error {
	a.targetMutex.Lock()
	defer a.targetMutex.Unlock()
	a.target = spec.TargetConcurrency
	return nil
}

// Record a data point.
func (a *Autoscaler) Record(ctx context.Context, stat Stat) {
	if stat.Time == nil {
		logger := logging.FromContext(ctx)
		logger.Errorf("Missing time from stat: %+v", stat)
		return
	}

	a.statsMutex.Lock()
	defer a.statsMutex.Unlock()

	bucketKey := stat.Time.Truncate(bucketSize)
	bucket, ok := a.bucketed[bucketKey]
	if !ok {
		bucket = statsBucket{}
		a.bucketed[bucketKey] = bucket
	}
	bucket.add(&stat)
}

// Scale calculates the desired scale based on current statistics given the current time.
func (a *Autoscaler) Scale(ctx context.Context, now time.Time) (int32, bool) {
	logger := logging.FromContext(ctx)

	originalReadyPodsCount, err := resources.FetchReadyAddressCount(a.endpointsLister, a.namespace, a.revisionService)
	if err != nil {
		logger.Errorw("Failed to get Endpoints via K8S Lister", zap.Error(err))
		return 0, false
	}
	// Use 1 if there are zero current pods.
	readyPodsCount := math.Max(1, float64(originalReadyPodsCount))

	config := a.Current()

	observedStableConcurrency, observedPanicConcurrency, lastBucket := a.aggregateData(now, config.StableWindow, config.PanicWindow)
	if len(a.bucketed) == 0 {
		logger.Debug("No data to scale on.")
		return 0, false
	}

	// Log system totals.
	logger.Debugf("Current concurrent clients: %0.3f", lastBucket.concurrency())

	target := a.targetConcurrency()
	// Desired pod count is observed concurrency of the revision over desired (stable) concurrency per pod.
	// The scaling up rate is limited to the MaxScaleUpRate.
	desiredStablePodCount := a.podCountLimited(math.Ceil(observedStableConcurrency/target), readyPodsCount)
	desiredPanicPodCount := a.podCountLimited(math.Ceil(observedPanicConcurrency/target), readyPodsCount)

	a.reporter.ReportStableRequestConcurrency(observedStableConcurrency)
	a.reporter.ReportPanicRequestConcurrency(observedPanicConcurrency)
	a.reporter.ReportTargetRequestConcurrency(target)

	logger.Debugf("STABLE: Observed average %0.3f concurrency over %v seconds.", observedStableConcurrency, config.StableWindow)
	logger.Debugf("PANIC: Observed average %0.3f concurrency over %v seconds.", observedPanicConcurrency, config.PanicWindow)

	isOverPanicThreshold := observedPanicConcurrency/readyPodsCount >= target*2

	if a.panicTime == nil && isOverPanicThreshold {
		// Begin panicking when we cross the concurrency threshold in the panic window.
		logger.Info("PANICKING")
		a.panicTime = &now
		a.reporter.ReportPanic(1)
	} else if a.panicTime != nil && !isOverPanicThreshold && a.panicTime.Add(config.StableWindow).Before(now) {
		// Stop panicking after the surge has made its way into the stable metric.
		logger.Info("Un-panicking.")
		a.panicTime = nil
		a.maxPanicPods = 0
		a.reporter.ReportPanic(0)
	}

	var desiredPodCount int32
	if a.panicTime != nil {
		logger.Debug("Operating in panic mode.")
		// We do not scale down while in panic mode. Only increases will be applied.
		if desiredPanicPodCount > a.maxPanicPods {
			logger.Infof("Increasing pods from %v to %v.", originalReadyPodsCount, desiredPanicPodCount)
			a.panicTime = &now
			a.maxPanicPods = desiredPanicPodCount
		}
		desiredPodCount = a.maxPanicPods
	} else {
		logger.Debug("Operating in stable mode.")
		desiredPodCount = desiredStablePodCount
	}

	a.reporter.ReportDesiredPodCount(int64(desiredPodCount))
	return desiredPodCount, true
}

// aggregateData aggregates bucketed stats over the stableWindow and panicWindow
// respectively and returns the observedStableConcurrency, observedPanicConcurrency
// and the last bucket that was aggregated.
func (a *Autoscaler) aggregateData(now time.Time, stableWindow, panicWindow time.Duration) (
	stableConcurrency float64, panicConcurrency float64, lastBucket statsBucket) {
	a.statsMutex.Lock()
	defer a.statsMutex.Unlock()

	var (
		stableBuckets float64
		stableTotal   float64

		panicBuckets float64
		panicTotal   float64

		lastBucketTime time.Time
	)
	for bucketTime, bucket := range a.bucketed {
		if !bucketTime.Add(panicWindow).Before(now) {
			panicBuckets++
			panicTotal += bucket.concurrency()
		}

		if !bucketTime.Add(stableWindow).Before(now) {
			stableBuckets++
			stableTotal += bucket.concurrency()
		} else {
			delete(a.bucketed, bucketTime)
		}

		if bucketTime.After(lastBucketTime) {
			lastBucketTime = bucketTime
			lastBucket = bucket
		}
	}

	if stableBuckets > 0 {
		stableConcurrency = stableTotal / stableBuckets
	}
	if panicBuckets > 0 {
		panicConcurrency = panicTotal / panicBuckets
	}

	return stableConcurrency, panicConcurrency, lastBucket
}

func (a *Autoscaler) targetConcurrency() float64 {
	a.targetMutex.RLock()
	defer a.targetMutex.RUnlock()
	return a.target
}

func (a *Autoscaler) podCountLimited(desiredPodCount, currentPodCount float64) int32 {
	return int32(math.Min(desiredPodCount, a.Current().MaxScaleUpRate*currentPodCount))
}
