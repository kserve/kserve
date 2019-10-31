package v1alpha2

import "fmt"

const (
	InvalidLoggerType = "Invalid logger type"
)

func validate_inference_logger(logger *Logger) error {
	if logger != nil {
		if !(logger.Mode == LogAll || logger.Mode == LogRequest || logger.Mode == LogResponse) {
			return fmt.Errorf(InvalidLoggerType)
		}
	}
	return nil
}
