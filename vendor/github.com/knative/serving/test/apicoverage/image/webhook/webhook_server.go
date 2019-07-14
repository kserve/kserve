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

package webhook

import (
	"container/list"
	"log"
	"net/http"

	"github.com/knative/pkg/signals"
	"github.com/knative/serving/test/apicoverage/image/common"
	"github.com/knative/serving/test/apicoverage/image/rules"
	"github.com/knative/test-infra/tools/webhook-apicoverage/resourcetree"
	"github.com/knative/test-infra/tools/webhook-apicoverage/webhook"
)

// SetupWebhookServer builds the necessary webhook configuration, HTTPServer and starts the webhook.
func SetupWebhookServer() {
	namespace := common.WebhookNamespace
	if len(namespace) == 0 {
		log.Fatal("Namespace value to used by the webhook is not set")
	}

	webhookConf := webhook.BuildWebhookConfiguration(common.CommonComponentName, common.CommonComponentName+".knative.serving.dev", common.WebhookNamespace)
	ac := webhook.APICoverageRecorder{
		Logger: webhookConf.Logger,
		ResourceForest: resourcetree.ResourceForest{
			Version:        "v1alpha1",
			ConnectedNodes: make(map[string]*list.List),
			TopLevelTrees:  make(map[string]resourcetree.ResourceTree),
		},
		ResourceMap:  common.ResourceMap,
		NodeRules:    rules.NodeRules,
		FieldRules:   rules.FieldRules,
		DisplayRules: rules.GetDisplayRules(),
	}
	ac.Init()

	m := http.NewServeMux()
	m.HandleFunc("/", ac.RecordResourceCoverage)
	m.HandleFunc(webhook.ResourceCoverageEndPoint, ac.GetResourceCoverage)
	m.HandleFunc(webhook.TotalCoverageEndPoint, ac.GetTotalCoverage)

	err := webhookConf.SetupWebhook(m, ac.ResourceMap, namespace, signals.SetupSignalHandler())
	if err != nil {
		log.Fatalf("Encountered error setting up Webhook: %v", err)
	}
}
