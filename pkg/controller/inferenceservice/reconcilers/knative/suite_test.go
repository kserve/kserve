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

package knative

import (
	"os"
	"testing"

	pkgtest "github.com/kubeflow/kfserving/pkg/testing"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

var cfg *rest.Config

func TestMain(m *testing.M) {
	t := pkgtest.SetupEnvTest()
	var err error
	if cfg, err = t.Start(); err != nil {
		klog.Fatal(err)
	}

	code := m.Run()
	t.Stop()
	os.Exit(code)
}
