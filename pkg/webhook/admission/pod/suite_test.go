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

	pkgtest "github.com/kserve/kserve/pkg/testing"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var cfg *rest.Config
var c client.Client

func TestMain(m *testing.M) {
	t := pkgtest.SetupEnvTest()
	var err error
	if cfg, err = t.Start(); err != nil {
		klog.Error(err, "Failed to start testing panel")
	}

	if c, err = client.New(cfg, client.Options{Scheme: scheme.Scheme}); err != nil {
		klog.Error(err, "Failed to start client")
	}
	code := m.Run()
	t.Stop()
	os.Exit(code)
}
