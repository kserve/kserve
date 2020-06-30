package v1beta1

import (
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

type TrainedModelStatus struct {
	// Condition for "Deployed"
	duckv1beta1.Status `json:",inline"`
	// Addressable endpoint for the deployed trained model
	// http://<inferenceservice.metadata.name>/v1/models/<trainedmodel>.metadata.name
	Address *duckv1beta1.Addressable `json:"address,omitempty"`
}
