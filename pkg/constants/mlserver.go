package constants

const (
	MLServerHTTPPortEnv            = "MLSERVER_HTTP_PORT"
	MLServerGRPCPortEnv            = "MLSERVER_GRPC_PORT"
	MLServerModelsDirEnv           = "MODELS_DIR"
	MLServerModelImplementationEnv = "MLSERVER_MODEL_IMPLEMENTATION"

	MLServerSKLearnImplementation = "mlserver.models.SKLearnModel"
	MLServerXGBoostImplementation = "mlserver.models.XGBoostModel"
)

var (
	MLServerISGRPCPort = int32(9000)
	MLServerISRestPort = int32(8080)
)
