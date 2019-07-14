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

package activator

import (
	"errors"
	"testing"

	"go.uber.org/zap"

	. "github.com/knative/pkg/logging/testing"
	"github.com/knative/pkg/test/helpers"
	nv1a1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/knative/serving/pkg/queue"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	revID   = RevisionID{"good-namespace", "good-name"}
	errTest = errors.New("some error")
)

const (
	defaultMaxConcurrency = 1000
	initCapacity          = 0
)

func existingRevisionGetter(concurrency v1beta1.RevisionContainerConcurrencyType) func(RevisionID) (*v1alpha1.Revision, error) {
	return func(RevisionID) (*v1alpha1.Revision, error) {
		return &v1alpha1.Revision{
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: concurrency,
				},
			},
		}, nil
	}
}
func erroringRevisionGetter(RevisionID) (*v1alpha1.Revision, error) {
	return nil, errTest
}

func existingEndpointsGetter(count int) EndpointsCountGetter {
	return func(*nv1a1.ServerlessService) (int, error) {
		return count, nil
	}
}
func erroringEndpointsCountGetter(*nv1a1.ServerlessService) (int, error) {
	return initCapacity, errTest
}

func sksGetError(string, string) (*nv1a1.ServerlessService, error) {
	return nil, errTest
}

func sksGetSuccess(namespace, name string) (*nv1a1.ServerlessService, error) {
	return &nv1a1.ServerlessService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Status: nv1a1.ServerlessServiceStatus{
			// Randomize the test.
			PrivateServiceName: helpers.AppendRandomString(name),
			ServiceName:        helpers.AppendRandomString(name),
		},
	}, nil
}

func TestThrottler_UpdateCapacity(t *testing.T) {
	samples := []struct {
		label          string
		revisionGetter RevisionGetter
		numEndpoints   int
		maxConcurrency int
		want           int
		wantError      error
	}{{
		label:          "all good",
		revisionGetter: existingRevisionGetter(10),
		numEndpoints:   1,
		maxConcurrency: defaultMaxConcurrency,
		want:           10,
	}, {
		label:          "unlimited concurrency",
		revisionGetter: existingRevisionGetter(0),
		numEndpoints:   1,
		maxConcurrency: 100,
		want:           100,
	}, {
		label:          "non-existing revision",
		revisionGetter: erroringRevisionGetter,
		numEndpoints:   1,
		maxConcurrency: defaultMaxConcurrency,
		want:           0,
		wantError:      errTest,
	}, {
		label:          "exceeds maxConcurrency",
		revisionGetter: existingRevisionGetter(10),
		numEndpoints:   1,
		maxConcurrency: 5,
		want:           5,
	}, {
		label:          "no endpoints",
		revisionGetter: existingRevisionGetter(1),
		numEndpoints:   0,
		maxConcurrency: 5,
		want:           0,
	}, {
		label:          "no endpoints, unlimited concurrency",
		revisionGetter: existingRevisionGetter(0),
		numEndpoints:   0,
		maxConcurrency: 5,
		want:           0,
	}}
	for _, s := range samples {
		t.Run(s.label, func(t *testing.T) {
			throttler := getThrottler(
				s.maxConcurrency, s.revisionGetter, nil, /*getEndpoints*/
				nil /*getSKS*/, TestLogger(t), initCapacity)
			err := throttler.UpdateCapacity(revID, s.numEndpoints)
			if got, want := err, s.wantError; got != want {
				t.Errorf("Update capacity error = %v, want: %v", got, want)
			}
			if s.want > 0 {
				if got := throttler.breakers[revID].Capacity(); got != s.want {
					t.Errorf("Breaker Capacity = %d, want: %d", got, s.want)
				}
			}
		})
	}
}

func TestThrottler_Try(t *testing.T) {
	defer ClearAll()
	samples := []struct {
		label           string
		addCapacity     bool
		wantCalls       int
		wantError       error
		revisionGetter  RevisionGetter
		endpointsGetter EndpointsCountGetter
		sksGetter       SKSGetter
	}{{
		label:           "all good",
		addCapacity:     true,
		wantCalls:       1,
		revisionGetter:  existingRevisionGetter(10),
		endpointsGetter: existingEndpointsGetter(0),
		sksGetter:       sksGetSuccess,
	}, {
		label:           "non-existing revision",
		addCapacity:     true,
		wantCalls:       0,
		revisionGetter:  erroringRevisionGetter,
		endpointsGetter: existingEndpointsGetter(0),
		sksGetter:       sksGetSuccess,
		wantError:       errTest,
	}, {
		label:           "error getting SKS",
		wantCalls:       0,
		revisionGetter:  existingRevisionGetter(10),
		endpointsGetter: existingEndpointsGetter(1),
		sksGetter:       sksGetError,
		wantError:       errTest,
	}, {
		label:           "error getting endpoint",
		wantCalls:       0,
		wantError:       errTest,
		revisionGetter:  existingRevisionGetter(10),
		endpointsGetter: erroringEndpointsCountGetter,
		sksGetter:       sksGetSuccess,
	}}
	for _, s := range samples {
		t.Run(s.label, func(t *testing.T) {
			var called int
			throttler := getThrottler(
				defaultMaxConcurrency, s.revisionGetter, s.endpointsGetter,
				s.sksGetter, TestLogger(t), initCapacity)
			if s.addCapacity {
				throttler.UpdateCapacity(revID, 1)
			}
			err := throttler.Try(revID, func() {
				called++
			})
			if got, want := err, s.wantError; got != want {
				t.Errorf("Update capacity error = %v, want: %v", got, want)
			}
			if got, want := called, s.wantCalls; got != want {
				t.Errorf("Unexpected number of function runs in Try = %d, want: %d", got, want)
			}
		})
	}
}

func TestThrottler_TryOverload(t *testing.T) {
	maxConcurrency := 1
	initialCapacity := 1
	queueLength := 1
	th := getThrottler(maxConcurrency, existingRevisionGetter(1),
		existingEndpointsGetter(1), sksGetSuccess,
		TestLogger(t), initialCapacity)

	doneCh := make(chan struct{})
	errCh := make(chan error)

	// Make one more request than allowed.
	allowedRequests := initialCapacity + queueLength
	for i := 0; i < allowedRequests+1; i++ {
		go func() {
			err := th.Try(revID, func() {
				doneCh <- struct{}{} // Blocks forever
			})
			if err != nil {
				errCh <- err
			}
		}()
	}

	if err := <-errCh; err != ErrActivatorOverload {
		t.Errorf("error = %v, want: %v", err, ErrActivatorOverload)
	}

	successfulRequests := 0
	for i := 0; i < allowedRequests; i++ {
		select {
		case <-doneCh:
			successfulRequests++
		case <-errCh:
			t.Errorf("Only one request should fail.")
		}
	}

	if successfulRequests != allowedRequests {
		t.Errorf("successfulRequests = %d, want: %d", successfulRequests, allowedRequests)
	}
}

func TestUpdateEndpoints(t *testing.T) {
	revisionConcurrency := 10

	samples := []struct {
		label          string
		endpointsAfter int
		wantCapacity   int
		initCapacity   int
	}{{
		label:          "add single endpoint",
		endpointsAfter: 1,
		wantCapacity:   1 * revisionConcurrency,
	}, {
		label:          "add several endpoints",
		endpointsAfter: 2,
		wantCapacity:   2 * revisionConcurrency,
	}, {
		label:          "do nothing",
		endpointsAfter: 1,
		wantCapacity:   1 * revisionConcurrency,
		initCapacity:   10,
	}, {
		label:          "reduce endpoints",
		endpointsAfter: 0,
		wantCapacity:   0,
		initCapacity:   10,
	}, {
		label:          "exceed max concurrency",
		endpointsAfter: 101 * revisionConcurrency,
		wantCapacity:   defaultMaxConcurrency,
	}}

	for _, s := range samples {
		t.Run(s.label, func(t *testing.T) {
			throttler := getThrottler(
				defaultMaxConcurrency, existingRevisionGetter(
					v1beta1.RevisionContainerConcurrencyType(revisionConcurrency)),
				existingEndpointsGetter(0), sksGetSuccess, TestLogger(t), s.initCapacity)
			breaker := queue.NewBreaker(throttler.breakerParams)
			throttler.breakers[revID] = breaker
			endpointsAfter := corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      helpers.AppendRandomString(revID.Name),
					Namespace: revID.Namespace,
				},
				Subsets: endpointsSubset(s.endpointsAfter, 1),
			}
			throttler.UpdateEndpoints(&endpointsAfter)
			if got := breaker.Capacity(); got != s.wantCapacity {
				t.Errorf("Breaker capacity = %d, want: %d", got, s.wantCapacity)
			}
		})
	}
}
func TestUpdateCapacityFail(t *testing.T) {
	throttler := getThrottler(
		defaultMaxConcurrency, erroringRevisionGetter, nil, nil,
		TestLogger(t), int(1))
	if got, want := throttler.UpdateCapacity(revID, 42), errTest; got != want {
		t.Errorf("UpdateCapacity error = %v, want: %v", got, want)
	}
}

func TestThrottler_Remove(t *testing.T) {
	throttler := getThrottler(
		defaultMaxConcurrency, existingRevisionGetter(10),
		existingEndpointsGetter(0), sksGetSuccess,
		TestLogger(t), initCapacity)
	throttler.breakers[revID] = queue.NewBreaker(throttler.breakerParams)

	if got := len(throttler.breakers); got != 1 {
		t.Errorf("Number of Breakers created = %d, want: 1", got)
	}
	throttler.Remove(revID)

	if got := len(throttler.breakers); got != 0 {
		t.Errorf("Number of Breakers created = %d, want: %d", got, 0)
	}
}

func TestHelper_DeleteBreaker(t *testing.T) {
	throttler := getThrottler(
		int(20), existingRevisionGetter(10),
		existingEndpointsGetter(0), sksGetSuccess,
		TestLogger(t), initCapacity)
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revID.Name + "-suffix",
			Namespace: revID.Namespace,
		},
	}
	revID := RevisionID{Namespace: revID.Namespace, Name: revID.Name}
	throttler.breakers[revID] = queue.NewBreaker(throttler.breakerParams)
	if got := len(throttler.breakers); got != 1 {
		t.Errorf("Breaker map size got %d, want: 1", got)
	}
	throttler.DeleteBreaker(endpoints)
	if len(throttler.breakers) != 0 {
		t.Errorf("Breaker map is not empty, got: %v", throttler.breakers)
	}
}

func getThrottler(
	maxConcurrency int, revisionGetter RevisionGetter,
	endpointsGetter EndpointsCountGetter, sksGetter SKSGetter,
	logger *zap.SugaredLogger,
	initCapacity int) *Throttler {
	params := queue.BreakerParams{
		QueueDepth:      1,
		MaxConcurrency:  maxConcurrency,
		InitialCapacity: initCapacity,
	}
	throttlerParams := ThrottlerParams{
		BreakerParams: params,
		Logger:        logger,
		GetRevision:   revisionGetter,
		GetEndpoints:  endpointsGetter,
		GetSKS:        sksGetter,
	}
	return NewThrottler(throttlerParams)
}

func endpointsSubset(hostsPerSubset, subsets int) []v1.EndpointSubset {
	resp := []v1.EndpointSubset{}
	if hostsPerSubset > 0 {
		addresses := make([]v1.EndpointAddress, hostsPerSubset)
		subset := v1.EndpointSubset{Addresses: addresses}
		for s := 0; s < subsets; s++ {
			resp = append(resp, subset)
		}
		return resp
	}
	return resp
}
