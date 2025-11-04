/*
Copyright 2022 The KServe Authors.

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

import corev1 "k8s.io/api/core/v1"

// BuildS3EnvVars sets s3 related env variables based on the provided configuration.
// Env variables will not be set unless their corresponding configured value is a non-empty string.
//
// Parameters:
//   - annotations: The annotations present within a service account or secret.
//   - secretData: The data contained within a secret, if needed.
//   - s3Config: The s3 configuration defined in the inferenceservice-config configmap.
//
// Returns:
//
//	A list of all set env variables.
func BuildS3EnvVars(annotations map[string]string, secretData *map[string][]byte, s3Config *S3Config) []corev1.EnvVar {
	envs := []corev1.EnvVar{}

	s3UseHttps := getEnvValue(annotations, secretData, InferenceServiceS3SecretHttpsAnnotation, S3UseHttps, s3Config.S3UseHttps)
	if s3UseHttps != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  S3UseHttps,
			Value: s3UseHttps,
		})
	}

	s3Endpoint := getEnvValue(annotations, secretData, InferenceServiceS3SecretEndpointAnnotation, S3Endpoint, s3Config.S3Endpoint)
	if s3Endpoint != "" {
		s3EndpointUrl := "https://" + s3Endpoint
		if s3UseHttps == "0" {
			s3EndpointUrl = "http://" + s3Endpoint
		}
		envs = append(envs, corev1.EnvVar{
			Name:  S3Endpoint,
			Value: s3Endpoint,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  AWSEndpointUrl,
			Value: s3EndpointUrl,
		})
	}

	s3VerifySSL := getEnvValue(annotations, secretData, InferenceServiceS3SecretSSLAnnotation, S3VerifySSL, s3Config.S3VerifySSL)
	if s3VerifySSL != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  S3VerifySSL,
			Value: s3VerifySSL,
		})
	}

	s3UseAnonymousCredential := getEnvValue(annotations, secretData, InferenceServiceS3UseAnonymousCredential, AWSAnonymousCredential, s3Config.S3UseAnonymousCredential)
	if s3UseAnonymousCredential != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  AWSAnonymousCredential,
			Value: s3UseAnonymousCredential,
		})
	}

	s3Region := getEnvValue(annotations, secretData, InferenceServiceS3SecretRegionAnnotation, AWSRegion, s3Config.S3Region)
	if s3Region != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  AWSRegion,
			Value: s3Region,
		})
	}

	s3UseVirtualBucket := getEnvValue(annotations, secretData, InferenceServiceS3UseVirtualBucketAnnotation, S3UseVirtualBucket, s3Config.S3UseVirtualBucket)
	if s3UseVirtualBucket != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  S3UseVirtualBucket,
			Value: s3UseVirtualBucket,
		})
	}

	s3UseAccelerate := getEnvValue(annotations, secretData, InferenceServiceS3UseAccelerateAnnotation, S3UseAccelerate, s3Config.S3UseAccelerate)
	if s3UseAccelerate != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  S3UseAccelerate,
			Value: s3UseAccelerate,
		})
	}

	s3CustomCABundle := getEnvValue(annotations, secretData, InferenceServiceS3CABundleAnnotation, AWSCABundle, s3Config.S3CABundle)
	if s3CustomCABundle != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  AWSCABundle,
			Value: s3CustomCABundle,
		})
	}

	s3CustomCABundleConfigMap := getEnvValue(annotations, secretData, InferenceServiceS3CABundleConfigMapAnnotation, AWSCABundleConfigMap, s3Config.S3CABundleConfigMap)
	if s3CustomCABundleConfigMap != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  AWSCABundleConfigMap,
			Value: s3CustomCABundleConfigMap,
		})
	}

	return envs
}

// getEnvValue fetches a value from the provided configuration.
// The value is fetched from the annotations, secretData, or s3Config in that order.
//
// Parameters:
//   - annotations: The annotations present within a service account or secret.
//   - secretData: The data contained within a secret, if needed.
//   - annotationKey: The key in the annotations map from which to fetch the desired env variable.
//   - secretDataKey: The key in the secret data map from which to fetch the desired env variable.
//   - s3ConfigValue: The value configured in the s3Config for the desired env variable.
//
// Returns:
//
//	The value fetched from the configuration if found, otherwise an empty string.
func getEnvValue(annotations map[string]string, secretData *map[string][]byte, annotationKey string, secretDataKey string, s3ConfigValue string) string {
	var envValue string
	if annotationValue, ok := annotations[annotationKey]; ok {
		envValue = annotationValue
	} else if secretValue, ok := getSecretValueFromPtr(secretData, secretDataKey); ok {
		envValue = secretValue
	} else {
		envValue = s3ConfigValue
	}

	return envValue
}

func getSecretValueFromPtr(secretData *map[string][]byte, secretDataKey string) (string, bool) {
	var found bool
	var secretValue string
	if secretData != nil {
		if val, ok := (*secretData)[secretDataKey]; ok {
			found = true
			secretValue = string(val)
		}
	}

	return secretValue, found
}
