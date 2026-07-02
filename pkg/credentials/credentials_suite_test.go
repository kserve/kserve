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

package credentials

import (
	"os"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgtest "github.com/kserve/kserve/pkg/testing"
)

var (
	cfg       *rest.Config
	c         client.Client
	clientset kubernetes.Interface
)

func TestMain(m *testing.M) {
	t := pkgtest.NewEnvTest().BuildEnvironment()
	var err error
	if cfg, err = t.Start(); err != nil {
		log.Error(err, "Failed to start testing panel")
	}

	if c, err = client.New(cfg, client.Options{Scheme: t.Scheme}); err != nil {
		log.Error(err, "Failed to start client")
	}
	if clientset, err = kubernetes.NewForConfig(cfg); err != nil {
		log.Error(err, "Failed to create clientset")
	}

	code := m.Run()
	_ = t.Stop()
	os.Exit(code)
}
