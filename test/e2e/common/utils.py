# Copyright 2019 kubeflow.org.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
import time
import logging
import json
import requests
from urllib.parse import urlparse
from kubernetes import client, config
from kfserving import KFServingClient

logging.basicConfig(level=logging.INFO)

KFServing = KFServingClient(config_file="~/.kube/config")
KFSERVING_NAMESPACE = "kfserving-system"
KFSERVING_TEST_NAMESPACE = "kfserving-ci-e2e-test"


def predict(service_name, input_json):
    isvc = KFServing.get(service_name, namespace=KFSERVING_TEST_NAMESPACE)
    # temporary sleep until this is fixed https://github.com/kubeflow/kfserving/issues/604
    time.sleep(10)
    cluster_ip = get_cluster_ip()
    host = urlparse(isvc['status']['url']).netloc
    url = "http://{}/v1/models/{}:predict".format(cluster_ip, service_name)
    headers = {'Host': host}
    with open(input_json) as json_file:
        data = json.load(json_file)
        logging.info("Sending request data: %s", json.dumps(data))
        response = requests.post(url, json.dumps(data), headers=headers)
        logging.info("Got response code %s, content %s", response.status_code, response.content)
        probs = json.loads(response.content.decode('utf-8'))["predictions"]
        return probs

def explain(service_name, input_json):
    isvc = KFServing.get(service_name, namespace=KFSERVING_TEST_NAMESPACE)
    # temporary sleep until this is fixed https://github.com/kubeflow/kfserving/issues/604
    time.sleep(10)
    cluster_ip = get_cluster_ip()
    host = urlparse(isvc['status']['url']).netloc
    url = "http://{}/v1/models/{}:explain".format(cluster_ip, service_name)
    headers = {'Host': host}
    with open(input_json) as json_file:
        data = json.load(json_file)
        logging.info("Sending request data: %s", json.dumps(data))
        logging.info("Print pod info before explain")
        pods = KFServing.core_api.list_namespaced_pod(KFSERVING_TEST_NAMESPACE,
               label_selector='serving.kubeflow.org/inferenceservice={}'.format(service_name))
        for pod in pods.items:
            logging.info(pod)
        try: 
            response = requests.post(url, json.dumps(data), headers=headers)
            logging.info("Got response code %s, content %s", response.status_code, response.content)
            precision = json.loads(response.content.decode('utf-8'))["precision"]
        except RuntimeError as e:
            logging.info("Explain runtime error -------")
            logging.info(KFServing.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1", KFSERVING_TEST_NAMESPACE,
                                                                      "services", service_name + "-explainer-default"))
            pods = KFServing.core_api.list_namespaced_pod(KFSERVING_TEST_NAMESPACE,
                                                        label_selector='serving.kubeflow.org/inferenceservice={}'.
                                                        format(service_name))
            for pod in pods.items:
                logging.info(pod)
                logging.info("%s\t%s\t%s" % (pod.metadata.name, 
                                             pod.status.phase,
                                             pod.status.pod_ip))
                api_response = KFServing.core_api.read_namespaced_pod_log(pod.metadata.name, KFSERVING_TEST_NAMESPACE)
                pprint(api_response) 
            raise e
        return precision

def get_cluster_ip():
    api_instance = client.CoreV1Api(client.ApiClient())
    service = api_instance.read_namespaced_service("istio-ingressgateway", "istio-system", exact='true')
    if service.status.load_balancer.ingress is None:
       cluster_ip = service.spec.cluster_ip
    else:
       cluster_ip = service.status.load_balancer.ingress[0].ip
    return cluster_ip
