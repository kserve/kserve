package v1alpha2

import "fmt"

const (
	InvalidInferenceLoggerSample = "Inference logger sample must be between 0 and 1"
)

func validate_inference_logger(logger *InferenceLogger) error {
	if logger != nil {
		if logger.Sample != nil {
			if *logger.Sample < 0.0 || *logger.Sample > 1.0 {
				return fmt.Errorf(InvalidInferenceLoggerSample)
			}
		}
	}
	return nil
}
