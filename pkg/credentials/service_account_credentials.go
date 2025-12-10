/*
Copyright 2021 The KServe Authors.

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
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials/azure"
	"github.com/kserve/kserve/pkg/credentials/gcs"
	"github.com/kserve/kserve/pkg/credentials/hdfs"
	"github.com/kserve/kserve/pkg/credentials/hf"
	"github.com/kserve/kserve/pkg/credentials/https"
	"github.com/kserve/kserve/pkg/credentials/s3"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	CredentialConfigKeyName     = "credentials"
	UriSchemePlaceholder        = "<scheme-placeholder>"
	StorageConfigEnvKey         = "STORAGE_CONFIG"
	StorageOverrideConfigEnvKey = "STORAGE_OVERRIDE_CONFIG"
	DefaultStorageSecretKey     = "default"
	UnsupportedStorageSpecType  = "storage type must be one of [%s]. storage type [%s] is not supported"
	MissingBucket               = "format [%s] requires a bucket but one wasn't found in storage data or parameters"
	AwsIrsaAnnotationKey        = "eks.amazonaws.com/role-arn"
)

var (
	SupportedStorageSpecTypes = []string{"s3", "hdfs", "webhdfs"}
	StorageBucketTypes        = []string{"s3"}
)

type CredentialConfig struct {
	S3                          s3.S3Config   `json:"s3,omitempty"`
	GCS                         gcs.GCSConfig `json:"gcs,omitempty"`
	StorageSpecSecretName       string        `json:"storageSpecSecretName,omitempty"`
	StorageSecretNameAnnotation string        `json:"storageSecretNameAnnotation,omitempty"`
}

type CredentialBuilder struct {
	client    client.Client
	clientset kubernetes.Interface
	config    CredentialConfig
}

var log = logf.Log.WithName("CredentialBuilder")

func GetCredentialConfig(configMap *corev1.ConfigMap) (CredentialConfig, error) {
	credentialConfig := CredentialConfig{}
	if credential, ok := configMap.Data[CredentialConfigKeyName]; ok {
		err := json.Unmarshal([]byte(credential), &credentialConfig)
		if err != nil {
			return credentialConfig, fmt.Errorf("unable to parse credential config json: %w", err)
		}
	}
	return credentialConfig, nil
}

func NewCredentialBuilder(client client.Client, clientset kubernetes.Interface, configMap *corev1.ConfigMap) *CredentialBuilder {
	credentialConfig, err := GetCredentialConfig(configMap)
	if err != nil {
		panic(err)
	}
	return NewCredentialBuilderFromConfig(client, clientset, credentialConfig)
}

func NewCredentialBuilderFromConfig(client client.Client, clientset kubernetes.Interface, config CredentialConfig) *CredentialBuilder {
	return &CredentialBuilder{
		client:    client,
		clientset: clientset,
		config:    config,
	}
}

func (c *CredentialBuilder) CreateStorageSpecSecretEnvs(namespace string, annotations map[string]string, storageKey string,
	overrideParams map[string]string, container *corev1.Container,
) error {
	stype := overrideParams["type"]
	bucket := overrideParams["bucket"]

	storageSecretName := constants.DefaultStorageSpecSecret
	if c.config.StorageSpecSecretName != "" {
		storageSecretName = c.config.StorageSpecSecretName
	}
	// secret annotation takes precedence
	if annotations != nil {
		if secretName, ok := annotations[c.config.StorageSecretNameAnnotation]; ok {
			storageSecretName = secretName
		}
	}
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(context.TODO(), storageSecretName, metav1.GetOptions{})

	var storageData []byte
	if err == nil {
		if storageKey != "" {
			if storageData = secret.Data[storageKey]; storageData == nil {
				return fmt.Errorf("specified storage key %s not found in storage secret %s",
					storageKey, storageSecretName)
			}
		} else {
			if stype == "" {
				storageKey = DefaultStorageSecretKey
			} else {
				storageKey = fmt.Sprintf("%s_%s", DefaultStorageSecretKey, stype)
			}
			// It's ok for the entry not to be found in the default/fallback cases
			storageData = secret.Data[storageKey]
		}
	} else if storageKey != "" || !apierr.IsNotFound(err) { // Don't fail if not found and no storage key was specified
		return fmt.Errorf("can't read storage secret %s: %w", storageSecretName, err)
	}

	if storageData != nil {
		if stype == "" {
			var storageDataJson map[string]string
			if err := json.Unmarshal(storageData, &storageDataJson); err != nil {
				return fmt.Errorf("invalid json encountered in key %s of storage secret %s: %w",
					storageKey, storageSecretName, err)
			}
			if storageType, ok := storageDataJson["type"]; ok {
				stype = storageType
				if !utils.Includes(SupportedStorageSpecTypes, stype) {
					return fmt.Errorf(UnsupportedStorageSpecType, strings.Join(SupportedStorageSpecTypes, ", "), stype)
				}
			}
			// Get bucket from storage-config if not provided in override params
			if _, ok := storageDataJson["bucket"]; ok && bucket == "" {
				bucket = storageDataJson["bucket"]
			}
			if cabundle_configmap, ok := storageDataJson["cabundle_configmap"]; ok {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  s3.AWSCABundleConfigMap,
					Value: cabundle_configmap,
				})
			}
		}

		// Pass storage config json as SecretKeyRef env var
		container.Env = append(container.Env, corev1.EnvVar{
			Name: StorageConfigEnvKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: storageSecretName,
					},
					Key: storageKey,
				},
			},
		})
	}

	if stype == "" {
		return errors.New("unable to determine storage type")
	}

	if strings.HasPrefix(container.Args[0], UriSchemePlaceholder+"://") {
		path := container.Args[0][len(UriSchemePlaceholder+"://"):]

		if utils.Includes(StorageBucketTypes, stype) {
			if bucket == "" {
				return fmt.Errorf(MissingBucket, stype)
			}

			container.Args[0] = fmt.Sprintf("%s://%s/%s", stype, bucket, path)
		} else {
			container.Args[0] = fmt.Sprintf("%s://%s", stype, path)
		}
	}

	// Provide override secret values if parameters are provided
	if len(overrideParams) != 0 {
		if overrideParamsJSON, err := json.Marshal(overrideParams); err == nil {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  StorageOverrideConfigEnvKey,
				Value: string(overrideParamsJSON),
			})
		}
	}

	return nil
}

func (c *CredentialBuilder) CreateSecretVolumeAndEnv(ctx context.Context, namespace string, annotations map[string]string, serviceAccountName string,
	container *corev1.Container, volumes *[]corev1.Volume,
) error {
	if serviceAccountName == "" {
		serviceAccountName = "default"
	}
	serviceAccount, err := c.clientset.CoreV1().ServiceAccounts(namespace).Get(ctx, serviceAccountName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to find service account", "ServiceAccountName", serviceAccountName,
			"Namespace", namespace)
		return nil
	}

	for annotationKey := range serviceAccount.Annotations {
		if annotationKey == AwsIrsaAnnotationKey {
			log.Info("AWS IAM Role annotation found, setting service account envs for s3", "ServiceAccountName", serviceAccountName)
			envs := s3.BuildServiceAccountEnvs(serviceAccount, &c.config.S3)
			container.Env = append(container.Env, envs...)
		}
	}

	// secret name annotation takes precedence
	if annotations != nil && c.config.StorageSecretNameAnnotation != "" {
		if secretName, ok := annotations[c.config.StorageSecretNameAnnotation]; ok {
			err := c.mountSecretCredential(ctx, secretName, namespace, container, volumes)
			if err != nil {
				log.Error(err, "Failed to amount the secret credentials", "secretName", secretName)
				return err
			}
			return nil
		}
	}

	// Find the secret references from service account
	for _, secretRef := range serviceAccount.Secrets {
		err := c.mountSecretCredential(ctx, secretRef.Name, namespace, container, volumes)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *CredentialBuilder) mountSecretCredential(ctx context.Context, secretName string, namespace string,
	container *corev1.Container, volumes *[]corev1.Volume,
) error {
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to find secret", "SecretName", secretName)
		return err
	} else {
		log.Info("found secret", "SecretName", secretName)
	}
	s3SecretAccessKeyName := s3.AWSSecretAccessKeyName
	gcsCredentialFileName := gcs.GCSCredentialFileName

	if c.config.S3.S3SecretAccessKeyName != "" {
		s3SecretAccessKeyName = c.config.S3.S3SecretAccessKeyName
	}

	if c.config.GCS.GCSCredentialFileName != "" {
		gcsCredentialFileName = c.config.GCS.GCSCredentialFileName
	}
	if _, ok := secret.Data[s3SecretAccessKeyName]; ok {
		log.Info("Setting secret envs for s3", "S3Secret", secret.Name)
		envs := s3.BuildSecretEnvs(secret, &c.config.S3)
		// Merge envs here to override values possibly present from IAM Role annotations with values from secret annotations
		container.Env = utils.MergeEnvs(container.Env, envs)
	} else if _, ok := secret.Data[gcsCredentialFileName]; ok {
		log.Info("Setting secret volume for gcs", "GCSSecret", secret.Name)
		volume, volumeMount := gcs.BuildSecretVolume(secret)
		*volumes = utils.AppendVolumeIfNotExists(*volumes, volume)
		container.VolumeMounts = append(container.VolumeMounts, volumeMount)
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  gcs.GCSCredentialEnvKey,
				Value: gcs.GCSCredentialVolumeMountPath + gcsCredentialFileName,
			})
	} else if _, ok := secret.Data[azure.LegacyAzureClientId]; ok {
		log.Info("Setting secret envs for azure", "AzureSecret", secret.Name)
		envs := azure.BuildSecretEnvs(secret)
		container.Env = append(container.Env, envs...)
	} else if _, ok := secret.Data[azure.AzureClientId]; ok {
		log.Info("Setting secret envs for azure", "AzureSecret", secret.Name)
		envs := azure.BuildSecretEnvs(secret)
		container.Env = append(container.Env, envs...)
	} else if _, ok := secret.Data[azure.AzureStorageAccessKey]; ok {
		log.Info("Setting secret envs with azure storage access key for azure", "AzureSecret", secret.Name)
		envs := azure.BuildStorageAccessKeySecretEnv(secret)
		container.Env = append(container.Env, envs...)
	} else if _, ok := secret.Data[https.HTTPSHost]; ok {
		log.Info("Setting secret volume from uri", "HTTP(S)Secret", secret.Name)
		envs := https.BuildSecretEnvs(secret)
		container.Env = append(container.Env, envs...)
	} else if _, ok := secret.Data[hdfs.HdfsNamenode]; ok {
		log.Info("Setting secret for hdfs", "HdfsSecret", secret.Name)
		volume, volumeMount := hdfs.BuildSecret(secret)
		*volumes = utils.AppendVolumeIfNotExists(*volumes, volume)
		container.VolumeMounts = append(container.VolumeMounts, volumeMount)
	} else if _, ok := secret.Data[hf.HFTokenKey]; ok {
		log.Info("Setting secret envs for huggingface", "HfSecret", secret.Name)
		envs := hf.BuildSecretEnvs(secret)
		container.Env = append(container.Env, envs...)
	} else {
		log.V(5).Info("Skipping unsupported secret", "Secret", secret.Name)
	}
	return nil
}
