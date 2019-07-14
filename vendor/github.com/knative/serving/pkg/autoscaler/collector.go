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
	"sync"
	"time"

	kpa "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// scrapeTickInterval is the interval of time between scraping metrics across
	// all pods of a revision.
	// TODO(yanweiguo): tuning this value. To be based on pod population?
	scrapeTickInterval = time.Second / 3
)

// Metric represents a resource to configure the metric collector with.
// +k8s:deepcopy-gen=true
type Metric struct {
	metav1.ObjectMeta
	Spec   MetricSpec
	Status MetricStatus
}

// MetricSpec contains all values the metric collector needs to operate.
type MetricSpec struct{}

// MetricStatus reflects the status of metric collection for this specific entity.
type MetricStatus struct{}

// StatsScraperFactory creates a StatsScraper for a given Metric.
type StatsScraperFactory func(*Metric) (StatsScraper, error)

// MetricCollector manages collection of metrics for many entities.
type MetricCollector struct {
	logger *zap.SugaredLogger

	statsScraperFactory StatsScraperFactory
	statsCh             chan *StatMessage

	collections      map[string]*collection
	collectionsMutex sync.RWMutex
}

// NewMetricCollector creates a new metric collector.
func NewMetricCollector(statsScraperFactory StatsScraperFactory, statsCh chan *StatMessage, logger *zap.SugaredLogger) *MetricCollector {
	collector := &MetricCollector{
		logger:              logger,
		collections:         make(map[string]*collection),
		statsScraperFactory: statsScraperFactory,
		statsCh:             statsCh,
	}

	return collector
}

// Get gets a Metric's state from the collector.
// Returns a copy of the Metric object. Mutations won't be seen by the collector.
func (c *MetricCollector) Get(ctx context.Context, namespace, name string) (*Metric, error) {
	c.collectionsMutex.RLock()
	defer c.collectionsMutex.RUnlock()

	key := NewMetricKey(namespace, name)
	collector, ok := c.collections[key]
	if !ok {
		return nil, k8serrors.NewNotFound(kpa.Resource("Metrics"), key)
	}

	return collector.metric.DeepCopy(), nil
}

// Create creates a new metric and thus starts collection for that entity.
// Returns a copy of the Metric object. Mutations won't be seen by the collector.
func (c *MetricCollector) Create(ctx context.Context, metric *Metric) (*Metric, error) {
	c.collectionsMutex.Lock()
	defer c.collectionsMutex.Unlock()

	c.logger.Debugf("Starting metric collection of %s/%s", metric.Namespace, metric.Name)

	key := NewMetricKey(metric.Namespace, metric.Name)
	coll, exists := c.collections[key]
	if !exists {
		scraper, err := c.statsScraperFactory(metric)
		if err != nil {
			return nil, err
		}
		coll = newCollection(metric, scraper, c.statsCh, c.logger)
		c.collections[key] = coll
	}

	return coll.metric.DeepCopy(), nil
}

// Update updates the Metric.
// Returns a copy of the Metric object. Mutations won't be seen by the collector.
func (c *MetricCollector) Update(ctx context.Context, metric *Metric) (*Metric, error) {
	c.collectionsMutex.Lock()
	defer c.collectionsMutex.Unlock()

	key := NewMetricKey(metric.Namespace, metric.Name)
	if collection, exists := c.collections[key]; exists {
		collection.metric = metric
		return collection.metric.DeepCopy(), nil
	}
	return nil, k8serrors.NewNotFound(kpa.Resource("Metrics"), key)
}

// Delete deletes a Metric and halts collection.
func (c *MetricCollector) Delete(ctx context.Context, namespace, name string) error {
	c.collectionsMutex.Lock()
	defer c.collectionsMutex.Unlock()

	c.logger.Debugf("Stopping metric collection of %s/%s", namespace, name)

	key := NewMetricKey(namespace, name)
	if collection, ok := c.collections[key]; ok {
		collection.close()
		delete(c.collections, key)
	}
	return nil
}

// collection represents the collection of metrics for one specific entity.
type collection struct {
	metric *Metric

	grp    sync.WaitGroup
	stopCh chan struct{}
}

func newCollection(metric *Metric, scraper StatsScraper, statsCh chan *StatMessage, logger *zap.SugaredLogger) *collection {
	c := &collection{
		metric: metric,
		stopCh: make(chan struct{}),
	}

	c.grp.Add(1)
	go func() {
		defer c.grp.Done()

		scrapeTicker := time.NewTicker(scrapeTickInterval)
		for {
			select {
			case <-c.stopCh:
				scrapeTicker.Stop()
				return
			case <-scrapeTicker.C:
				message, err := scraper.Scrape()
				if err != nil {
					logger.Errorw("Failed to scrape metrics", zap.Error(err))
				}
				if message != nil {
					statsCh <- message
				}
			}
		}
	}()

	return c
}

func (c *collection) close() {
	close(c.stopCh)
	c.grp.Wait()
}
