/*
Copyright 2019 kubeflow.org.

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

package testing

import (
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"path/filepath"
	"sync"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("TestingEnvSetup")

func SetupEnvTest() *envtest.Environment {
	t := &envtest.Environment{
		// The relative paths must be provided for each level of test nesting
		// This code should be illegal
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "..", "config", "default", "crds", "base"),
			filepath.Join("..", "..", "..", "..", "..", "test", "crds"),
			filepath.Join("..", "..", "..", "config", "default", "crds", "base"),
			filepath.Join("..", "..", "..", "test", "crds"),
		},
	}

	err := v1alpha2.SchemeBuilder.AddToScheme(scheme.Scheme)

	if err != nil {
		log.Error(err, "Failed to add kfserving scheme")
	}

	if err = knservingv1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add knative serving scheme")
	}

	if err = v1alpha3.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add istio scheme")
	}
	return t
}

// SetupTestReconcile returns a reconcile.Reconcile implementation that delegates to inner and
// writes the request to requests after Reconcile is finished.
func SetupTestReconcile(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request) {
	requests := make(chan reconcile.Request)
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		result, err := inner.Reconcile(req)
		requests <- req
		return result, err
	})
	return fn, requests
}

// StartTestManager adds recFn
func StartTestManager(mgr manager.Manager, g *gomega.GomegaWithT) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(stop)).NotTo(gomega.HaveOccurred())
	}()
	return stop, wg
}
