/*
Copyright 2019 The Knative Authors.

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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	. "github.com/knative/pkg/logging/testing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	defaultNamespace = "test-namespace"
	defaultName      = "test-name"
	defaultMetric    = &Metric{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: defaultNamespace,
			Name:      defaultName,
		},
	}
)

func TestMetricCollectorCrud(t *testing.T) {
	defer ClearAll()

	logger := TestLogger(t)
	ctx := context.Background()

	scraper := testScraper(func() (*StatMessage, error) {
		return nil, nil
	})
	factory := scraperFactory(scraper, nil)
	statsCh := make(chan *StatMessage)

	t.Run("error on mismatch", func(t *testing.T) {
		coll := NewMetricCollector(factory, statsCh, logger)
		coll.Create(ctx, &Metric{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "another-namespace",
				Name:      defaultName,
			},
		})
		if _, err := coll.Get(ctx, defaultNamespace, defaultName); err == nil {
			t.Error("Get() did not return an error")
		}
		if _, err := coll.Update(ctx, defaultMetric); err == nil {
			t.Error("Update() did not return an error")
		}
	})

	t.Run("error on creating scraper", func(t *testing.T) {
		want := errors.New("factory failure")
		failingFactory := scraperFactory(nil, want)

		coll := NewMetricCollector(failingFactory, statsCh, logger)
		_, got := coll.Create(ctx, defaultMetric)

		if got != want {
			t.Errorf("Create() = %v, want %v", got, want)
		}
	})

	t.Run("full crud", func(t *testing.T) {
		coll := NewMetricCollector(factory, statsCh, logger)
		coll.Create(ctx, defaultMetric)

		got, err := coll.Get(ctx, defaultNamespace, defaultName)
		if err != nil {
			t.Errorf("Get() = %v, want no error", err)
		}
		if !cmp.Equal(defaultMetric, got) {
			t.Errorf("Get() didn't return the same metric: %v", cmp.Diff(defaultMetric, got))
		}

		got, err = coll.Update(ctx, defaultMetric)
		if err != nil {
			t.Errorf("Update() = %v, want no error", err)
		}
		if !cmp.Equal(defaultMetric, got) {
			t.Errorf("Update() didn't return the same metric: %v", cmp.Diff(defaultMetric, got))
		}

		if err := coll.Delete(ctx, defaultNamespace, defaultName); err != nil {
			t.Errorf("Delete() = %v, want no error", err)
		}
	})
}

func TestMetricCollectorScraper(t *testing.T) {
	defer ClearAll()

	logger := TestLogger(t)
	ctx := context.Background()

	want := &StatMessage{
		Key: NewMetricKey(defaultNamespace, defaultName),
		Stat: Stat{
			PodName: "testPod",
		},
	}
	scraper := testScraper(func() (*StatMessage, error) {
		return want, nil
	})
	factory := scraperFactory(scraper, nil)
	statsCh := make(chan *StatMessage)

	coll := NewMetricCollector(factory, statsCh, logger)
	coll.Create(ctx, defaultMetric)

	got := <-statsCh
	if got != want {
		t.Errorf("<-statsCh = %v, want %v", got, want)
	}

	coll.Delete(ctx, defaultNamespace, defaultName)
	select {
	case <-time.After(scrapeTickInterval * 2):
		// All good!
	case <-statsCh:
		t.Error("Got unexpected metric after stopping collection")
	}
}

func scraperFactory(scraper StatsScraper, err error) StatsScraperFactory {
	return func(*Metric) (StatsScraper, error) {
		return scraper, err
	}
}

type testScraper func() (*StatMessage, error)

func (s testScraper) Scrape() (*StatMessage, error) {
	return s()
}
