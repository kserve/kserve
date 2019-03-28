package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServiceStatus struct {
	Conditions                metav1.ConditionsSpec `json:"conditions,omitempty"`
	Uri                       UriSpec               `json:"uri,omitempty"`
	Revisions                 []RevisionsSpec       `json:"revisions,omitempty"`
	latestCreatedRevisionName string                `json:"latestCreatedRevisionName,omitempty"`
	latestReadyRevisionName   string                `json:"latestReadyRevisionName,omitempty"`
}

type UriSpec struct {
	Internal string `json:"internal,omitempty"`
	External string `json:"external,omitempty"`
}

type RevisionsSpec struct {
	Name     string `json:"name,omitempty"`
	Replicas int    `json:"replicas,omitempty"`
	Traffic  int    `json:"traffic,omitempty"`
}
