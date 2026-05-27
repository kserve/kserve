/*
Copyright 2026 The KServe Authors.

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

package config

const (
	envEnableGPU       = "ENABLE_GPU"
	envSkipPrecheck    = "SKIP_PRECHECK"
	envEnableBaremetal = "ENABLE_BAREMETAL"
	envEnableSTUB      = "ENABLE_STUB"
	envKubeConfig      = "KUBE_CONFIG"
	envMCVNamespace    = "MCV_NAMESPACE"

	defaultNamespace  = "mcv"
	defaultKubeConfig = ""
	defaultConfDir    = "/tmp/mcv/"
	defaultConfFile   = "mcv.config"
	GPU               = "gpu"
)

var ConfDir string = "/tmp/mcv/"
var ConfFile string = "mcv.config"
