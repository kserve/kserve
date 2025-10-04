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

import (
	corev1 "k8s.io/api/core/v1"
)

/*
For a quick reference about AWS ENV variables:
AWS Cli: https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html
Boto: https://boto3.amazonaws.com/v1/documentation/api/latest/guide/configuration.html#using-environment-variables
*/

func BuildServiceAccountEnvs(serviceAccount *corev1.ServiceAccount, s3Config *S3Config) []corev1.EnvVar {
	envs := []corev1.EnvVar{}

	envs = append(envs, BuildS3EnvVars(serviceAccount.Annotations, nil, s3Config)...)

	return envs
}
