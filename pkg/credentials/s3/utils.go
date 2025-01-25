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

func BuildS3EnvVars(annotations map[string]string, s3Config *S3Config) []corev1.EnvVar {
	envs := []corev1.EnvVar{}

	if s3Endpoint, ok := annotations[InferenceServiceS3SecretEndpointAnnotation]; ok {
		s3EndpointUrl := "https://" + s3Endpoint
		if s3UseHttps, ok := annotations[InferenceServiceS3SecretHttpsAnnotation]; ok {
			if s3UseHttps == "0" {
				s3EndpointUrl = "http://" + annotations[InferenceServiceS3SecretEndpointAnnotation]
			}
			envs = append(envs, corev1.EnvVar{
				Name:  S3UseHttps,
				Value: s3UseHttps,
			})
		}
		envs = append(envs, corev1.EnvVar{
			Name:  S3Endpoint,
			Value: s3Endpoint,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  AWSEndpointUrl,
			Value: s3EndpointUrl,
		})
	} else if s3Config.S3Endpoint != "" {
		s3EndpointUrl := "https://" + s3Config.S3Endpoint
		if s3Config.S3UseHttps == "0" {
			s3EndpointUrl = "http://" + s3Config.S3Endpoint
			envs = append(envs, corev1.EnvVar{
				Name:  S3UseHttps,
				Value: s3Config.S3UseHttps,
			})
		}
		envs = append(envs, corev1.EnvVar{
			Name:  S3Endpoint,
			Value: s3Config.S3Endpoint,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  AWSEndpointUrl,
			Value: s3EndpointUrl,
		})
	}

	// For each variable, prefer the value from the annotation, otherwise default to the value from the inferenceservice configmap if set.
	verifySsl, ok := annotations[InferenceServiceS3SecretSSLAnnotation]
	if !ok {
		verifySsl = s3Config.S3VerifySSL
	}
	if verifySsl != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  S3VerifySSL,
			Value: verifySsl,
		})
	}

	useAnonymousCredential, ok := annotations[InferenceServiceS3UseAnonymousCredential]
	if !ok {
		useAnonymousCredential = s3Config.S3UseAnonymousCredential
	}
	if useAnonymousCredential != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  AWSAnonymousCredential,
			Value: useAnonymousCredential,
		})
	}

	s3Region, ok := annotations[InferenceServiceS3SecretRegionAnnotation]
	if !ok {
		s3Region = s3Config.S3Region
	}
	if s3Region != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  AWSRegion,
			Value: s3Region,
		})
	}

	useVirtualBucket, ok := annotations[InferenceServiceS3UseVirtualBucketAnnotation]
	if !ok {
		useVirtualBucket = s3Config.S3UseVirtualBucket
	}
	if useVirtualBucket != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  S3UseVirtualBucket,
			Value: useVirtualBucket,
		})
	}

	useAccelerate, ok := annotations[InferenceServiceS3UseAccelerateAnnotation]
	if !ok {
		useAccelerate = s3Config.S3UseAccelerate
	}
	if useAccelerate != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  S3UseAccelerate,
			Value: useAccelerate,
		})
	}

	customCABundle, ok := annotations[InferenceServiceS3CABundleAnnotation]
	if !ok {
		customCABundle = s3Config.S3CABundle
	}
	if customCABundle != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  AWSCABundle,
			Value: customCABundle,
		})
	}

	customCABundleConfigMap, ok := annotations[InferenceServiceS3CABundleConfigMapAnnotation]
	if !ok {
		customCABundleConfigMap = s3Config.S3CABundleConfigMap
	}
	if customCABundleConfigMap != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  AWSCABundleConfigMap,
			Value: customCABundleConfigMap,
		})
	}

	return envs
}
