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

package inject

import (
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var instance *testSource
var uninjectable *failSource
var errInjectFail = fmt.Errorf("injection fails")
var expectedFalse = false

var _ = Describe("runtime inject", func() {

	BeforeEach(func() {
		instance = &testSource{}
		uninjectable = &failSource{}
	})

	It("should set informers", func() {
		injectedCache := &informertest.FakeInformers{}

		By("Validating injecting the informer")
		res, err := CacheInto(injectedCache, instance)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(true))
		Expect(injectedCache).To(Equal(instance.GetCache()))

		By("Returing false if the type does not implement inject.Cache")
		res, err = CacheInto(injectedCache, uninjectable)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(expectedFalse))
		Expect(uninjectable.GetCache()).To(BeNil())

		By("Returning an error if informer injection fails")
		res, err = CacheInto(nil, instance)
		Expect(err).To(Equal(errInjectFail))
		Expect(res).To(Equal(true))

	})

	It("should set config", func() {

		cfg := &rest.Config{}

		By("Validating injecting config")
		res, err := ConfigInto(cfg, instance)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(true))
		Expect(cfg).To(Equal(instance.GetConfig()))

		By("Returning false if the type does not implement inject.Config")
		res, err = ConfigInto(cfg, uninjectable)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(false))
		Expect(uninjectable.GetConfig()).To(BeNil())

		By("Returning an error if config injection fails")
		res, err = ConfigInto(nil, instance)
		Expect(err).To(Equal(errInjectFail))
		Expect(res).To(Equal(true))
	})

	It("should set client", func() {
		client := client.DelegatingClient{}

		By("Validating injecting client")
		res, err := ClientInto(client, instance)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(true))
		Expect(client).To(Equal(instance.GetClient()))

		By("Returning false if the type does not implement inject.Client")
		res, err = ClientInto(client, uninjectable)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(false))
		Expect(uninjectable.GetClient()).To(BeNil())

		By("Returning an error if client injection fails")
		res, err = ClientInto(nil, instance)
		Expect(err).To(Equal(errInjectFail))
		Expect(res).To(Equal(true))
	})

	It("should set scheme", func() {

		scheme := runtime.NewScheme()

		By("Validating injecting scheme")
		res, err := SchemeInto(scheme, instance)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(true))
		Expect(scheme).To(Equal(instance.GetScheme()))

		By("Returning false if the type does not implement inject.Scheme")
		res, err = SchemeInto(scheme, uninjectable)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(false))
		Expect(uninjectable.GetScheme()).To(BeNil())

		By("Returning an error if scheme injection fails")
		res, err = SchemeInto(nil, instance)
		Expect(err).To(Equal(errInjectFail))
		Expect(res).To(Equal(true))
	})

	It("should set stop channel", func() {

		stop := make(<-chan struct{})

		By("Validating injecting stop channel")
		res, err := StopChannelInto(stop, instance)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(true))
		Expect(stop).To(Equal(instance.GetStop()))

		By("Returning false if the type does not implement inject.Stoppable")
		res, err = StopChannelInto(stop, uninjectable)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(false))
		Expect(uninjectable.GetStop()).To(BeNil())

		By("Returning an error if stop channel injection fails")
		res, err = StopChannelInto(nil, instance)
		Expect(err).To(Equal(errInjectFail))
		Expect(res).To(Equal(true))
	})

	It("should set dependencies", func() {

		f := func(interface{}) error { return nil }

		By("Validating injecting dependencies")
		res, err := InjectorInto(f, instance)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(true))
		Expect(reflect.ValueOf(f).Pointer()).To(Equal(reflect.ValueOf(instance.GetFunc()).Pointer()))

		By("Returning false if the type does not implement inject.Injector")
		res, err = InjectorInto(f, uninjectable)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(false))
		Expect(uninjectable.GetFunc()).To(BeNil())

		By("Returning an error if dependencies injection fails")
		res, err = InjectorInto(nil, instance)
		Expect(err).To(Equal(errInjectFail))
		Expect(res).To(Equal(true))
	})

})

type testSource struct {
	scheme *runtime.Scheme
	cache  cache.Cache
	config *rest.Config
	client client.Client
	f      Func
	stop   <-chan struct{}
}

func (s *testSource) InjectCache(c cache.Cache) error {
	if c != nil {
		s.cache = c
		return nil
	}
	return fmt.Errorf("injection fails")
}

func (s *testSource) InjectConfig(config *rest.Config) error {
	if config != nil {
		s.config = config
		return nil
	}
	return fmt.Errorf("injection fails")
}

func (s *testSource) InjectClient(client client.Client) error {
	if client != nil {
		s.client = client
		return nil
	}
	return fmt.Errorf("injection fails")
}

func (s *testSource) InjectScheme(scheme *runtime.Scheme) error {
	if scheme != nil {
		s.scheme = scheme
		return nil
	}
	return fmt.Errorf("injection fails")
}

func (s *testSource) InjectStopChannel(stop <-chan struct{}) error {
	if stop != nil {
		s.stop = stop
		return nil
	}
	return fmt.Errorf("injection fails")
}

func (s *testSource) InjectFunc(f Func) error {
	if f != nil {
		s.f = f
		return nil
	}
	return fmt.Errorf("injection fails")
}

func (s *testSource) GetCache() cache.Cache {
	return s.cache
}

func (s *testSource) GetConfig() *rest.Config {
	return s.config
}

func (s *testSource) GetScheme() *runtime.Scheme {
	return s.scheme
}

func (s *testSource) GetClient() client.Client {
	return s.client
}

func (s *testSource) GetFunc() Func {
	return s.f
}

func (s *testSource) GetStop() <-chan struct{} {
	return s.stop
}

type failSource struct {
	scheme *runtime.Scheme
	cache  cache.Cache
	config *rest.Config
	client client.Client
	f      Func
	stop   <-chan struct{}
}

func (s *failSource) GetCache() cache.Cache {
	return s.cache
}

func (s *failSource) GetConfig() *rest.Config {
	return s.config
}

func (s *failSource) GetScheme() *runtime.Scheme {
	return s.scheme
}

func (s *failSource) GetClient() client.Client {
	return s.client
}

func (s *failSource) GetFunc() Func {
	return s.f
}

func (s *failSource) GetStop() <-chan struct{} {
	return s.stop
}
