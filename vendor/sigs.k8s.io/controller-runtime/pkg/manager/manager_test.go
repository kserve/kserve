/*
Copyright 2018 The Kubernetes Authors.

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

package manager

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/leaderelection"
	fakeleaderelection "sigs.k8s.io/controller-runtime/pkg/leaderelection/fake"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/recorder"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

var _ = Describe("manger.Manager", func() {
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
	})

	AfterEach(func() {
		close(stop)
	})

	Describe("New", func() {
		It("should return an error if there is no Config", func() {
			m, err := New(nil, Options{})
			Expect(m).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("must specify Config"))

		})

		It("should return an error if it can't create a RestMapper", func() {
			expected := fmt.Errorf("expected error: RestMapper")
			m, err := New(cfg, Options{
				MapperProvider: func(c *rest.Config) (meta.RESTMapper, error) { return nil, expected },
			})
			Expect(m).To(BeNil())
			Expect(err).To(Equal(expected))

		})

		It("should return an error it can't create a client.Client", func(done Done) {
			m, err := New(cfg, Options{
				newClient: func(config *rest.Config, options client.Options) (client.Client, error) {
					return nil, fmt.Errorf("expected error")
				},
			})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))

			close(done)
		})

		It("should return an error it can't create a cache.Cache", func(done Done) {
			m, err := New(cfg, Options{
				newCache: func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
					return nil, fmt.Errorf("expected error")
				},
			})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))

			close(done)
		})
		It("should return an error it can't create a recorder.Provider", func(done Done) {
			m, err := New(cfg, Options{
				newRecorderProvider: func(config *rest.Config, scheme *runtime.Scheme, logger logr.Logger) (recorder.Provider, error) {
					return nil, fmt.Errorf("expected error")
				},
			})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))

			close(done)
		})
		Context("with leader election enabled", func() {
			It("should default ID to controller-runtime if ID is not set", func() {
				var rl resourcelock.Interface
				m, err := New(cfg, Options{
					LeaderElection:          true,
					LeaderElectionNamespace: "default",
					newResourceLock: func(config *rest.Config, recorderProvider recorder.Provider, options leaderelection.Options) (resourcelock.Interface, error) {
						var err error
						rl, err = leaderelection.NewResourceLock(config, recorderProvider, options)
						return rl, err
					},
				})
				Expect(m).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())
				Expect(rl.Describe()).To(Equal("default/controller-leader-election-helper"))
			})

			It("should return an error if namespace not set and not running in cluster", func() {
				m, err := New(cfg, Options{LeaderElection: true, LeaderElectionID: "controller-runtime"})
				Expect(m).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to find leader election namespace: not running in-cluster, please specify LeaderElectionNamespace"))
			})
		})

		It("should create a listener for the metrics if a valid address is provided", func() {
			var listener net.Listener
			m, err := New(cfg, Options{
				MetricsBindAddress: ":0",
				newMetricsListener: func(addr string) (net.Listener, error) {
					var err error
					listener, err = metrics.NewListener(addr)
					return listener, err
				},
			})
			Expect(m).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
			Expect(listener).ToNot(BeNil())
			Expect(listener.Close()).ToNot(HaveOccurred())
		})

		It("should return an error if the metrics bind address is already in use", func() {
			ln, err := metrics.NewListener(":0")
			Expect(err).ShouldNot(HaveOccurred())

			var listener net.Listener
			m, err := New(cfg, Options{
				MetricsBindAddress: ln.Addr().String(),
				newMetricsListener: func(addr string) (net.Listener, error) {
					var err error
					listener, err = metrics.NewListener(addr)
					return listener, err
				},
			})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(listener).To(BeNil())

			Expect(ln.Close()).ToNot(HaveOccurred())
		})
	})

	Describe("Start", func() {
		var startSuite = func(options Options) {
			It("should Start each Component", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				c1 := make(chan struct{})
				m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer close(c1)
					defer GinkgoRecover()
					return nil
				}))

				c2 := make(chan struct{})
				m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer close(c2)
					defer GinkgoRecover()
					return nil
				}))

				go func() {
					defer GinkgoRecover()
					Expect(m.Start(stop)).NotTo(HaveOccurred())
				}()
				<-c1
				<-c2

				close(done)
			})

			It("should stop when stop is called", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				s := make(chan struct{})
				close(s)
				Expect(m.Start(s)).NotTo(HaveOccurred())

				close(done)
			})

			It("should return an error if it can't start the cache", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				mgr, ok := m.(*controllerManager)
				Expect(ok).To(BeTrue())
				mgr.startCache = func(stop <-chan struct{}) error {
					return fmt.Errorf("expected error")
				}
				Expect(m.Start(stop).Error()).To(ContainSubstring("expected error"))

				close(done)
			})

			It("should return an error if any Components fail to Start", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				c1 := make(chan struct{})
				m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					defer close(c1)
					return nil
				}))

				c2 := make(chan struct{})
				m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					defer close(c2)
					return fmt.Errorf("expected error")
				}))

				c3 := make(chan struct{})
				m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					defer close(c3)
					return nil
				}))

				go func() {
					defer GinkgoRecover()
					Expect(m.Start(stop)).NotTo(HaveOccurred())
					close(done)
				}()
				<-c1
				<-c2
				<-c3
			})
		}

		Context("with defaults", func() {
			startSuite(Options{})
		})

		Context("with leaderelection enabled", func() {
			startSuite(Options{
				LeaderElection:          true,
				LeaderElectionID:        "controller-runtime",
				LeaderElectionNamespace: "default",
				newResourceLock:         fakeleaderelection.NewResourceLock,
			})
		})

		Context("should start serving metrics", func() {
			var listener net.Listener
			var opts Options

			BeforeEach(func() {
				listener = nil
				opts = Options{
					newMetricsListener: func(addr string) (net.Listener, error) {
						var err error
						listener, err = metrics.NewListener(addr)
						return listener, err
					},
				}
			})

			AfterEach(func() {
				if listener != nil {
					listener.Close()
				}
			})

			It("should stop serving metrics when stop is called", func(done Done) {
				opts.MetricsBindAddress = ":0"
				m, err := New(cfg, opts)
				Expect(err).NotTo(HaveOccurred())

				s := make(chan struct{})
				go func() {
					defer GinkgoRecover()
					Expect(m.Start(s)).NotTo(HaveOccurred())
					close(done)
				}()

				// Check the metrics started
				endpoint := fmt.Sprintf("http://%s", listener.Addr().String())
				_, err = http.Get(endpoint)
				Expect(err).NotTo(HaveOccurred())

				// Shutdown the server
				close(s)

				// Expect the metrics server to shutdown
				Eventually(func() error {
					_, err = http.Get(endpoint)
					return err
				}).ShouldNot(Succeed())
			})

			It("should serve metrics endpoint", func(done Done) {
				opts.MetricsBindAddress = ":0"
				m, err := New(cfg, opts)
				Expect(err).NotTo(HaveOccurred())

				s := make(chan struct{})
				defer close(s)
				go func() {
					defer GinkgoRecover()
					Expect(m.Start(s)).NotTo(HaveOccurred())
					close(done)
				}()

				metricsEndpoint := fmt.Sprintf("http://%s/metrics", listener.Addr().String())
				resp, err := http.Get(metricsEndpoint)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
			})

			It("should not serve anything other than metrics endpoint", func(done Done) {
				opts.MetricsBindAddress = ":0"
				m, err := New(cfg, opts)
				Expect(err).NotTo(HaveOccurred())

				s := make(chan struct{})
				defer close(s)
				go func() {
					defer GinkgoRecover()
					Expect(m.Start(s)).NotTo(HaveOccurred())
					close(done)
				}()

				endpoint := fmt.Sprintf("http://%s/should-not-exist", listener.Addr().String())
				resp, err := http.Get(endpoint)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})

			It("should serve metrics in its registry", func(done Done) {
				one := prometheus.NewCounter(prometheus.CounterOpts{
					Name: "test_one",
					Help: "test metric for testing",
				})
				one.Inc()
				err := metrics.Registry.Register(one)
				Expect(err).NotTo(HaveOccurred())

				opts.MetricsBindAddress = ":0"
				m, err := New(cfg, opts)
				Expect(err).NotTo(HaveOccurred())

				s := make(chan struct{})
				defer close(s)
				go func() {
					defer GinkgoRecover()
					Expect(m.Start(s)).NotTo(HaveOccurred())
					close(done)
				}()

				metricsEndpoint := fmt.Sprintf("http://%s/metrics", listener.Addr().String())
				resp, err := http.Get(metricsEndpoint)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				data, err := ioutil.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).To(ContainSubstring("%s\n%s\n%s\n",
					`# HELP test_one test metric for testing`,
					`# TYPE test_one counter`,
					`test_one 1`,
				))

				// Unregister will return false if the metric was never registered
				ok := metrics.Registry.Unregister(one)
				Expect(ok).To(BeTrue())
			})
		})
	})

	Describe("Add", func() {
		It("should immediately start the Component if the Manager has already Started another Component",
			func(done Done) {
				m, err := New(cfg, Options{})
				Expect(err).NotTo(HaveOccurred())
				mgr, ok := m.(*controllerManager)
				Expect(ok).To(BeTrue())

				// Add one component before starting
				c1 := make(chan struct{})
				m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer close(c1)
					defer GinkgoRecover()
					return nil
				}))

				go func() {
					defer GinkgoRecover()
					Expect(m.Start(stop)).NotTo(HaveOccurred())
				}()

				// Wait for the Manager to start
				Eventually(func() bool { return mgr.started }).Should(BeTrue())

				// Add another component after starting
				c2 := make(chan struct{})
				m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer close(c2)
					defer GinkgoRecover()
					return nil
				}))
				<-c1
				<-c2

				close(done)
			})

		It("should immediately start the Component if the Manager has already Started", func(done Done) {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			mgr, ok := m.(*controllerManager)
			Expect(ok).To(BeTrue())

			go func() {
				defer GinkgoRecover()
				Expect(m.Start(stop)).NotTo(HaveOccurred())
			}()

			// Wait for the Manager to start
			Eventually(func() bool { return mgr.started }).Should(BeTrue())

			c1 := make(chan struct{})
			m.Add(RunnableFunc(func(s <-chan struct{}) error {
				defer close(c1)
				defer GinkgoRecover()
				return nil
			}))
			<-c1

			close(done)
		})

		It("should fail if SetFields fails", func() {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(m.Add(&failRec{})).To(HaveOccurred())
		})
	})
	Describe("SetFields", func() {
		It("should inject field values", func(done Done) {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			mgr, ok := m.(*controllerManager)
			Expect(ok).To(BeTrue())

			mgr.cache = &informertest.FakeInformers{}

			By("Injecting the dependencies")
			err = m.SetFields(&injectable{
				scheme: func(scheme *runtime.Scheme) error {
					defer GinkgoRecover()
					Expect(scheme).To(Equal(m.GetScheme()))
					return nil
				},
				config: func(config *rest.Config) error {
					defer GinkgoRecover()
					Expect(config).To(Equal(m.GetConfig()))
					return nil
				},
				client: func(client client.Client) error {
					defer GinkgoRecover()
					Expect(client).To(Equal(m.GetClient()))
					return nil
				},
				cache: func(c cache.Cache) error {
					defer GinkgoRecover()
					Expect(c).To(Equal(m.GetCache()))
					return nil
				},
				stop: func(stop <-chan struct{}) error {
					defer GinkgoRecover()
					Expect(stop).NotTo(BeNil())
					return nil
				},
				f: func(f inject.Func) error {
					defer GinkgoRecover()
					Expect(f).NotTo(BeNil())
					return nil
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Returning an error if dependency injection fails")

			expected := fmt.Errorf("expected error")
			err = m.SetFields(&injectable{
				client: func(client client.Client) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				scheme: func(scheme *runtime.Scheme) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				config: func(config *rest.Config) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				cache: func(c cache.Cache) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				f: func(c inject.Func) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))
			err = m.SetFields(&injectable{
				stop: func(<-chan struct{}) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))
			close(done)
		})
	})

	It("should provide a function to get the Config", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		mgr, ok := m.(*controllerManager)
		Expect(ok).To(BeTrue())
		Expect(m.GetConfig()).To(Equal(mgr.config))
	})

	It("should provide a function to get the Client", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		mgr, ok := m.(*controllerManager)
		Expect(ok).To(BeTrue())
		Expect(m.GetClient()).To(Equal(mgr.client))
	})

	It("should provide a function to get the Scheme", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		mgr, ok := m.(*controllerManager)
		Expect(ok).To(BeTrue())
		Expect(m.GetScheme()).To(Equal(mgr.scheme))
	})

	It("should provide a function to get the FieldIndexer", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		mgr, ok := m.(*controllerManager)
		Expect(ok).To(BeTrue())
		Expect(m.GetFieldIndexer()).To(Equal(mgr.fieldIndexes))
	})

	It("should provide a function to get the EventRecorder", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		Expect(m.GetRecorder("test")).NotTo(BeNil())
	})
})

var _ reconcile.Reconciler = &failRec{}
var _ inject.Client = &failRec{}

type failRec struct{}

func (*failRec) Reconcile(reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (*failRec) Start(<-chan struct{}) error {
	return nil
}

func (*failRec) InjectClient(client.Client) error {
	return fmt.Errorf("expected error")
}

var _ inject.Injector = &injectable{}
var _ inject.Cache = &injectable{}
var _ inject.Client = &injectable{}
var _ inject.Scheme = &injectable{}
var _ inject.Config = &injectable{}
var _ inject.Stoppable = &injectable{}

type injectable struct {
	scheme func(scheme *runtime.Scheme) error
	client func(client.Client) error
	config func(config *rest.Config) error
	cache  func(cache.Cache) error
	f      func(inject.Func) error
	stop   func(<-chan struct{}) error
}

func (i *injectable) InjectCache(c cache.Cache) error {
	if i.cache == nil {
		return nil
	}
	return i.cache(c)
}

func (i *injectable) InjectConfig(config *rest.Config) error {
	if i.config == nil {
		return nil
	}
	return i.config(config)
}

func (i *injectable) InjectClient(c client.Client) error {
	if i.client == nil {
		return nil
	}
	return i.client(c)
}

func (i *injectable) InjectScheme(scheme *runtime.Scheme) error {
	if i.scheme == nil {
		return nil
	}
	return i.scheme(scheme)
}

func (i *injectable) InjectFunc(f inject.Func) error {
	if i.f == nil {
		return nil
	}
	return i.f(f)
}

func (i *injectable) InjectStopChannel(stop <-chan struct{}) error {
	if i.stop == nil {
		return nil
	}
	return i.stop(stop)
}

func (i *injectable) Start(<-chan struct{}) error {
	return nil
}
