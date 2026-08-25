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

package pod

import (
	"os"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgtest "github.com/kserve/kserve/pkg/testing"
)

var (
	cfg       *rest.Config
	c         client.Client
	clientset kubernetes.Interface
)

func TestMain(m *testing.M) {
	testEnv := pkgtest.NewEnvTest().BuildEnvironment()

	var err error
	if cfg, err = testEnv.Start(); err != nil {
		klog.Errorf("Failed to start test environment: %v", err)
		os.Exit(1)
	}

	if c, err = client.New(cfg, client.Options{Scheme: testEnv.Scheme}); err != nil {
		klog.Errorf("Failed to create client: %v", err)
		os.Exit(1)
	}

	if clientset, err = kubernetes.NewForConfig(cfg); err != nil {
		klog.Errorf("Failed to create clientset: %v", err)
		os.Exit(1)
	}

	code := m.Run()
	if err := testEnv.Stop(); err != nil {
		klog.Errorf("Failed to stop test environment: %v", err)
	}
	os.Exit(code)
}
