package v1alpha2

import "fmt"

const (
	InvalidLoggerType = "Invalid logger type"
)

func validate_inference_logger(logger *Logger) error {
	if logger != nil {
		if !(logger.LogType == LogAll || logger.LogType == LogRequest || logger.LogType == Logresponse) {
			return fmt.Errorf(InvalidLoggerType)
		}
	}
	return nil
}
