/*
Copyright 2021 The KServe Authors.

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
	"context"
	"sync"

	"google.golang.org/protobuf/proto"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	netv1 "k8s.io/api/networking/v1"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	operatorv1beta1 "knative.dev/operator/pkg/apis/operator/v1beta1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var log = logf.Log.WithName("TestingEnvSetup")

func SetupEnvTest(crdDirectoryPaths []string) *envtest.Environment {
	t := &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		// The relative paths must be provided for each level of test nesting
		// This code should be illegal
		CRDDirectoryPaths:  crdDirectoryPaths,
		UseExistingCluster: proto.Bool(false),
	}

	if err := netv1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add networking v1 scheme")
	}

	if err := knservingv1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add knative serving scheme")
	}

	if err := istioclientv1beta1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add istio scheme")
	}

	if err := operatorv1beta1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add knative operator scheme")
	}

	if err := gatewayapiv1.Install(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add gateway scheme")
	}
	if err := kedav1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add KEDA scheme")
	}
	return t
}

// StartTestManager adds recFn
func StartTestManager(ctx context.Context, mgr manager.Manager, g *gomega.GomegaWithT) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(ctx)).NotTo(gomega.HaveOccurred())
	}()
	return wg
}
