/*
Copyright 2019 The Knative Authors

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

package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/knative/serving/test/apicoverage/image/common"
	"github.com/knative/serving/test/apicoverage/image/rules"
	"github.com/knative/test-infra/shared/prow"
	"github.com/knative/test-infra/tools/webhook-apicoverage/tools"
)

func main() {
	var (
		kubeConfigPath string
		serviceIP      string
		err            error
	)

	// Ensure artifactsDir exist, in case not invoked from this script
	artifactsDir := prow.GetLocalArtifactsDir()
	if _, err := os.Stat(artifactsDir); os.IsNotExist(err) {
		if err = os.MkdirAll(artifactsDir, 0777); err != nil {
			log.Fatalf("Failed to create directory: %v", err)
		}
	}

	if kubeConfigPath, err = tools.GetDefaultKubePath(); err != nil {
		log.Fatalf("Error retrieving kubeConfig path: %v", err)
	}

	if serviceIP, err = tools.GetWebhookServiceIP(kubeConfigPath, "", common.WebhookNamespace, common.CommonComponentName); err != nil {
		log.Fatalf("Error retrieving Service IP: %v", err)
	}

	for resource := range common.ResourceMap {
		err = tools.GetAndWriteResourceCoverage(serviceIP, resource.Kind, path.Join(artifactsDir, strings.ToLower(resource.Kind)+".html"), rules.GetDisplayRules())
		if err != nil {
			log.Println(fmt.Sprintf("resource coverage for resource: %s failed. %v ", resource.Kind, err))
		}
	}

	if err := tools.GetAndWriteTotalCoverage(serviceIP, path.Join(artifactsDir, "totalcoverage.html")); err != nil {
		log.Fatalf("total coverage retrieval failed: %v", err)
	}
}
