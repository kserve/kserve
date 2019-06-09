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
	"encoding/json"
	"fmt"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/credentials/gcs"
	"github.com/kubeflow/kfserving/pkg/credentials/s3"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	CREDENTIAL_CONFIG_KEY_NAME = "credentials"
)

type CredentialConfig struct {
	S3  s3.S3Config   `json:"s3,omitempty"`
	GCS gcs.GCSConfig `json:"gcs,omitempty"`
}

type CredentialBuilder struct {
	client client.Client
	config CredentialConfig
}

var log = logf.Log.WithName("CredentialBulder")

func NewCredentialBulder(client client.Client, config *v1.ConfigMap) *CredentialBuilder {
	credentialConfig := CredentialConfig{}
	if credential, ok := config.Data[CREDENTIAL_CONFIG_KEY_NAME]; ok {
		err := json.Unmarshal([]byte(credential), &credentialConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall json string due to %v ", err))
		}
	}
	return &CredentialBuilder{
		client: client,
		config: credentialConfig,
	}
}

func (c *CredentialBuilder) CreateSecretVolumeAndEnv(ctx context.Context, namespace string, serviceAccountName string,
	configuration *knservingv1alpha1.Configuration) error {
	if serviceAccountName == "" {
		serviceAccountName = "default"
	}
	s3SecretAccessKeyName := s3.AWSSecretAccessKeyName
	gcsCredentialFileName := gcs.GCSCredentialFileName

	if c.config.S3.S3SecretAccessKeyName != "" {
		s3SecretAccessKeyName = c.config.S3.S3SecretAccessKeyName
	}

	if c.config.GCS.GCSCredentialFileName != "" {
		gcsCredentialFileName = c.config.GCS.GCSCredentialFileName
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
		if _, ok := secret.Data[s3SecretAccessKeyName]; ok {
			log.Info("Setting secret envs for s3", "S3Secret", secret.Name)
			envs := s3.BuildSecretEnvs(secret, &c.config.S3)
			configuration.Spec.RevisionTemplate.Spec.Container.Env = append(configuration.Spec.RevisionTemplate.Spec.Container.Env, envs...)
		} else if _, ok := secret.Data[gcsCredentialFileName]; ok {
			log.Info("Setting secret volume for gcs", "GCSSecret", secret.Name)
			volume, volumeMount := gcs.BuildSecretVolume(secret, gcsCredentialFileName)
			configuration.Spec.RevisionTemplate.Spec.Volumes =
				append(configuration.Spec.RevisionTemplate.Spec.Volumes, volume)
			configuration.Spec.RevisionTemplate.Spec.Container.VolumeMounts =
				append(configuration.Spec.RevisionTemplate.Spec.Container.VolumeMounts, volumeMount)
			configuration.Spec.RevisionTemplate.Spec.Container.Env = append(configuration.Spec.RevisionTemplate.Spec.Container.Env,
				v1.EnvVar{
					Name:  gcs.GCSCredentialEnvKey,
					Value: gcs.GCSCredentialVolumeMountPathPrefix + gcsCredentialFileName,
				})
		} else {
			log.V(5).Info("Skipping non gcs/s3 secret", "Secret", secret.Name)
		}
	}

	return nil
}
