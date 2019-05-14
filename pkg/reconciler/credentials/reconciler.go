/*
Copyright 2019 kubeflow.org.

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

package credentials

import (
	"context"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/reconciler/credentials/gcs"
	"github.com/kubeflow/kfserving/pkg/reconciler/credentials/s3"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("CredentialReconciler")

type CredentialReconciler struct {
	client client.Client
}

func NewCredentialReconciler(client client.Client) *CredentialReconciler {
	return &CredentialReconciler{
		client: client,
	}
}

func (c *CredentialReconciler) ReconcileServiceAccount(ctx context.Context, namespace string, serviceAccountName string,
	configuration *knservingv1alpha1.Configuration) error {
	if serviceAccountName == "" {
		serviceAccountName = "default"
	}
	serviceAccount := &v1.ServiceAccount{}
	err := c.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName,
		Namespace: namespace}, serviceAccount)
	if err != nil {
		log.Error(err, "Failed to find service account", "ServiceAccountName", serviceAccountName)
		return err
	}
	for _, secretRef := range serviceAccount.Secrets {
		secret := &v1.Secret{}
		err := c.client.Get(context.TODO(), types.NamespacedName{Name: secretRef.Name,
			Namespace: namespace}, secret)
		if err != nil {
			log.Error(err, "Failed to find secret", "SecretName", secretRef.Name)
			continue
		}
		if _, ok := secret.Annotations[constants.KFServiceS3SecretEndpointAnnotation]; ok {
			log.Info("Reconciling secret for s3", "S3Secret", secret.Name)
			s3.AttachSecretEnvsAndVolume(secret, configuration)
		} else if _, ok := secret.Annotations[constants.KFServiceGCSSecretAnnotation]; ok {
			log.Info("Reconciling secret for gcs", "GCSSecret", secret.Name)
			gcs.AttachSecretEnvsAndVolume(secret, configuration)
		}
	}

	return nil
}
