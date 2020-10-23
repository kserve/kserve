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
