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
	"fmt"

	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// InitializeConditions initializes the certificate conditions.
func (cs *CertificateStatus) InitializeConditions() {
	certificateCondSet.Manage(cs).InitializeConditions()
}

// MarkReady marks the certificate as ready to use.
func (cs *CertificateStatus) MarkReady() {
	certificateCondSet.Manage(cs).MarkTrue(CertificateConditionReady)
}

// MarkUnknown marks the certificate status as unknown.
func (cs *CertificateStatus) MarkUnknown(reason, message string) {
	certificateCondSet.Manage(cs).MarkUnknown(CertificateConditionReady, reason, message)
}

// MarkNotReady marks the certificate as not ready.
func (cs *CertificateStatus) MarkNotReady(reason, message string) {
	certificateCondSet.Manage(cs).MarkFalse(CertificateConditionReady, reason, message)
}

// MarkResourceNotOwned changes the ready condition to false to reflect that we don't own the
// resource of the given kind and name.
func (cs *CertificateStatus) MarkResourceNotOwned(kind, name string) {
	certificateCondSet.Manage(cs).MarkFalse(CertificateConditionReady, "NotOwned",
		fmt.Sprintf("There is an existing %s %q that we do not own.", kind, name))
}

// IsReady returns true is the Certificate is ready.
func (cs *CertificateStatus) IsReady() bool {
	return certificateCondSet.Manage(cs).IsHappy()
}

// GetCondition gets a speicifc condition of the Certificate status.
func (cs *CertificateStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return certificateCondSet.Manage(cs).GetCondition(t)
}

// ConditionType represents a Certificate condition value
const (
	// CertificateConditionReady is set when the requested certificate
	// is provioned and valid.
	CertificateConditionReady = apis.ConditionReady
)

var certificateCondSet = apis.NewLivingConditionSet(CertificateConditionReady)

// GetGroupVersionKind returns the GroupVersionKind of Certificate.
func (c *Certificate) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Certificate")
}

func (cs *CertificateStatus) duck() *duckv1beta1.Status {
	return &cs.Status
}
