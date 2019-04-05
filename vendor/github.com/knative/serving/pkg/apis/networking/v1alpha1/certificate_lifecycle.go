/*
Copyright 2019 The Knative Authors.

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

package v1alpha1

import (
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// InitializeConditions initializes the certificate conditions.
func (cs *CertificateStatus) InitializeConditions() {
	certificateCondSet.Manage(cs).InitializeConditions()
}

// MarkReady marks the certificate as ready to use.
func (cs *CertificateStatus) MarkReady() {
	certificateCondSet.Manage(cs).MarkTrue(CertificateCondidtionReady)
}

// IsReady returns true is the Certificate is ready.
func (cs *CertificateStatus) IsReady() bool {
	return certificateCondSet.Manage(cs).IsHappy()
}

// GetCondition gets a speicifc condition of the Certificate status.
func (cs *CertificateStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return certificateCondSet.Manage(cs).GetCondition(t)
}

// ConditionType represents a Certificate condition value
const (
	// CertificateConditionReady is set when the requested certificate
	// is provioned and valid.
	CertificateCondidtionReady = duckv1alpha1.ConditionReady
)

var certificateCondSet = duckv1alpha1.NewLivingConditionSet(CertificateCondidtionReady)

// GetGroupVersionKind returns the GroupVersionKind of Certificate.
func (c *Certificate) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Certificate")
}
