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

package s3

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
)

/*
For a quick reference about AWS ENV variables:
AWS Cli: https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html
Boto: https://boto3.amazonaws.com/v1/documentation/api/latest/guide/configuration.html#using-environment-variables
*/
const (
	AWSAccessKeyId         = "AWS_ACCESS_KEY_ID"
	AWSSecretAccessKey     = "AWS_SECRET_ACCESS_KEY" // #nosec G101
	AWSAccessKeyIdName     = "awsAccessKeyID"
	AWSSecretAccessKeyName = "awsSecretAccessKey"
	AWSEndpointUrl         = "AWS_ENDPOINT_URL"
	AWSRegion              = "AWS_DEFAULT_REGION"
	S3Endpoint             = "S3_ENDPOINT"
	S3UseHttps             = "S3_USE_HTTPS"
	S3VerifySSL            = "S3_VERIFY_SSL"
	S3UseVirtualBucket     = "S3_USER_VIRTUAL_BUCKET"
	S3UseAccelerate        = "S3_USE_ACCELERATE"
	AWSAnonymousCredential = "awsAnonymousCredential"
	AWSCABundle            = "AWS_CA_BUNDLE"
	AWSCABundleConfigMap   = "AWS_CA_BUNDLE_CONFIGMAP"
)

type S3Config struct {
	S3AccessKeyIDName        string `json:"s3AccessKeyIDName,omitempty"`
	S3SecretAccessKeyName    string `json:"s3SecretAccessKeyName,omitempty"`
	S3Endpoint               string `json:"s3Endpoint,omitempty"`
	S3UseHttps               string `json:"s3UseHttps,omitempty"`
	S3Region                 string `json:"s3Region,omitempty"`
	S3VerifySSL              string `json:"s3VerifySSL,omitempty"`
	S3UseVirtualBucket       string `json:"s3UseVirtualBucket,omitempty"`
	S3UseAccelerate          string `json:"s3UseAccelerate,omitempty"`
	S3UseAnonymousCredential string `json:"s3UseAnonymousCredential,omitempty"`
	S3CABundleConfigMap      string `json:"s3CABundleConfigMap,omitempty"`
	S3CABundle               string `json:"s3CABundle,omitempty"`
}

var (
	InferenceServiceS3SecretEndpointAnnotation    = constants.KServeAPIGroupName + "/" + "s3-endpoint"
	InferenceServiceS3SecretRegionAnnotation      = constants.KServeAPIGroupName + "/" + "s3-region"
	InferenceServiceS3SecretSSLAnnotation         = constants.KServeAPIGroupName + "/" + "s3-verifyssl"
	InferenceServiceS3SecretHttpsAnnotation       = constants.KServeAPIGroupName + "/" + "s3-usehttps"
	InferenceServiceS3UseVirtualBucketAnnotation  = constants.KServeAPIGroupName + "/" + "s3-usevirtualbucket"
	InferenceServiceS3UseAccelerateAnnotation     = constants.KServeAPIGroupName + "/" + "s3-useaccelerate"
	InferenceServiceS3UseAnonymousCredential      = constants.KServeAPIGroupName + "/" + "s3-useanoncredential"
	InferenceServiceS3CABundleConfigMapAnnotation = constants.KServeAPIGroupName + "/" + "s3-cabundle-configmap"
	InferenceServiceS3CABundleAnnotation          = constants.KServeAPIGroupName + "/" + "s3-cabundle"
)

func BuildSecretEnvs(secret *corev1.Secret, s3Config *S3Config) []corev1.EnvVar {
	s3SecretAccessKeyName := AWSSecretAccessKeyName
	s3AccessKeyIdName := AWSAccessKeyIdName
	if s3Config.S3AccessKeyIDName != "" {
		s3AccessKeyIdName = s3Config.S3AccessKeyIDName
	}

	if s3Config.S3SecretAccessKeyName != "" {
		s3SecretAccessKeyName = s3Config.S3SecretAccessKeyName
	}
	envs := []corev1.EnvVar{
		{
			Name: AWSAccessKeyId,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: s3AccessKeyIdName,
				},
			},
		},
		{
			Name: AWSSecretAccessKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: s3SecretAccessKeyName,
				},
			},
		},
	}

	envs = append(envs, BuildS3EnvVars(secret.Annotations, s3Config)...)

	return envs
}
