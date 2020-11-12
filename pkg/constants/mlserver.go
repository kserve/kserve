/*
Copyright 2020 kubeflow.org.

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

package constants

const (
	MLServerHTTPPortEnv            = "MLSERVER_HTTP_PORT"
	MLServerGRPCPortEnv            = "MLSERVER_GRPC_PORT"
	MLServerModelsDirEnv           = "MODELS_DIR"
	MLServerModelImplementationEnv = "MLSERVER_MODEL_IMPLEMENTATION"
	MLServerModelNameEnv           = "MLSERVER_MODEL_NAME"
	MLServerModelVersionEnv        = "MLSERVER_MODEL_VERSION"
	MLServerModelURIEnv            = "MLSERVER_MODEL_URI"

	MLServerSKLearnImplementation = "mlserver.models.SKLearnModel"
	MLServerXGBoostImplementation = "mlserver.models.XGBoostModel"

	MLServerModelVersionDefault = "v1"
)

var (
	MLServerISGRPCPort = int32(9000)
	MLServerISRestPort = int32(8080)
)
