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

package source_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Source Suite", []Reporter{envtest.NewlineReporter{}})
}

var testenv *envtest.Environment
var config *rest.Config
var clientset *kubernetes.Clientset
var icache cache.Cache
var stop chan struct{}

var _ = BeforeSuite(func(done Done) {
	stop = make(chan struct{})
	logf.SetLogger(logf.ZapLoggerTo(GinkgoWriter, true))

	testenv = &envtest.Environment{}

	var err error
	config, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	clientset, err = kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred())

	icache, err = cache.New(config, cache.Options{})
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		Expect(icache.Start(stop)).NotTo(HaveOccurred())
	}()

	close(done)
}, 60)

var _ = AfterSuite(func(done Done) {
	close(stop)
	testenv.Stop()

	close(done)
}, 5)
