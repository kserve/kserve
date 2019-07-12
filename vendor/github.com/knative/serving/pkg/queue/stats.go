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

package queue

import (
	"time"

	"github.com/knative/serving/pkg/autoscaler"
)

// ReqEvent represents either an incoming or closed request.
type ReqEvent struct {
	Time      time.Time
	EventType ReqEventType
}

// ReqEventType denotes the type (incoming/closed) of a ReqEvent.
type ReqEventType int

const (
	// ReqIn represents an incoming request
	ReqIn ReqEventType = iota
	// ReqOut represents a finished request
	ReqOut
	// ProxiedIn represents an incoming request through a proxy.
	ProxiedIn
	// ProxiedOut represents a finished proxied request.
	ProxiedOut
)

// Channels is a structure for holding the channels for driving Stats.
// It's just to make the NewStats signature easier to read.
type Channels struct {
	// Ticks with every request arrived/completed respectively
	ReqChan chan ReqEvent
	// Ticks with every stat report request
	ReportChan <-chan time.Time
	// Stat reporting channel
	StatChan chan *autoscaler.Stat
}

// Stats is a structure for holding channels per pod.
type Stats struct {
	podName string
	ch      Channels
}

// NewStats instantiates a new instance of Stats.
func NewStats(podName string, channels Channels, startedAt time.Time) *Stats {
	s := &Stats{
		podName: podName,
		ch:      channels,
	}

	go func() {
		var (
			requestCount       int32
			proxiedCount       int32
			concurrency        int32
			proxiedConcurrency int32
		)

		lastChange := startedAt
		timeOnConcurrency := make(map[int32]time.Duration)
		timeOnProxiedConcurrency := make(map[int32]time.Duration)

		// Updates the lastChanged/timeOnConcurrency state
		// Note: Due to nature of the channels used below, the ReportChan
		// can race the ReqChan, thus an event can arrive that has a lower
		// timestamp than `lastChange`. This is ignored, since it only makes
		// for very slight differences.
		updateState := func(time time.Time) {
			if time.After(lastChange) {
				durationSinceChange := time.Sub(lastChange)
				timeOnConcurrency[concurrency] += durationSinceChange
				timeOnProxiedConcurrency[proxiedConcurrency] += durationSinceChange
				lastChange = time
			}
		}

		for {
			select {
			case event := <-s.ch.ReqChan:
				updateState(event.Time)

				switch event.EventType {
				case ProxiedIn:
					proxiedConcurrency++
					proxiedCount++
					fallthrough
				case ReqIn:
					requestCount++
					concurrency++
				case ProxiedOut:
					proxiedConcurrency--
					fallthrough
				case ReqOut:
					concurrency--
				}
			case now := <-s.ch.ReportChan:
				updateState(now)

				stat := &autoscaler.Stat{
					Time:                             &now,
					PodName:                          s.podName,
					AverageConcurrentRequests:        weightedAverage(timeOnConcurrency),
					AverageProxiedConcurrentRequests: weightedAverage(timeOnProxiedConcurrency),
					RequestCount:                     requestCount,
					ProxiedRequestCount:              proxiedCount,
				}
				// Send the stat to another goroutine to transmit
				// so we can continue bucketing stats.
				s.ch.StatChan <- stat

				// Reset the stat counts which have been reported.
				timeOnConcurrency = make(map[int32]time.Duration)
				timeOnProxiedConcurrency = make(map[int32]time.Duration)
				requestCount = 0
				proxiedCount = 0
			}
		}
	}()

	return s
}

func weightedAverage(times map[int32]time.Duration) float64 {
	var totalTimeUsed time.Duration
	for _, val := range times {
		totalTimeUsed += val
	}
	avg := 0.0
	if totalTimeUsed > 0 {
		for c, val := range times {
			ratio := float64(val) / float64(totalTimeUsed)
			avg += float64(c) * ratio
		}
	}
	return avg
}
