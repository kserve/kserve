package v1alpha2

import "fmt"

const (
	InvalidLoggerType = "Invalid logger type"
)

func validateLogger(logger *Logger) error {
	if logger != nil {
		if !(logger.Mode == LogAll || logger.Mode == LogRequest || logger.Mode == LogResponse) {
			return fmt.Errorf(InvalidLoggerType)
		}
	}
	return nil
}
