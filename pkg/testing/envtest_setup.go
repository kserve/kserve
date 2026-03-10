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
	"google.golang.org/protobuf/proto"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	kservescheme "github.com/kserve/kserve/pkg/scheme"
)

var log = logf.Log.WithName("TestingEnvSetup")

func SetupEnvTest(crdDirectoryPaths []string) *envtest.Environment {
	t := &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     crdDirectoryPaths,
		UseExistingCluster:    proto.Bool(false),
	}

	if err := kservescheme.AddAll(scheme.Scheme); err != nil {
		log.Error(err, "Failed to register envtest schemes")
	}
	return t
}
