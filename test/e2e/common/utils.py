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
import os
from urllib.parse import urlparse
from kubernetes import client, config
from kfserving import KFServingClient
from kfserving import constants

logging.basicConfig(level=logging.INFO)

KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
KFSERVING_NAMESPACE = "kfserving-system"
KFSERVING_TEST_NAMESPACE = "kfserving-ci-e2e-test"


def predict(service_name, input_json, protocol_version="v1",
            version=constants.KFSERVING_V1BETA1_VERSION, model_name=None):
    isvc = KFServing.get(
        service_name,
        namespace=KFSERVING_TEST_NAMESPACE,
        version=version,
    )
    # temporary sleep until this is fixed https://github.com/kubeflow/kfserving/issues/604
    time.sleep(10)
    cluster_ip = get_cluster_ip()
    host = urlparse(isvc["status"]["url"]).netloc
    headers = {"Host": host}

    if model_name is None:
        model_name = service_name

    url = f"http://{cluster_ip}/v1/models/{model_name}:predict"
    if protocol_version == "v2":
        url = f"http://{cluster_ip}/v2/models/{model_name}/infer"

    with open(input_json) as json_file:
        data = json.load(json_file)
        logging.info("Sending request data: %s", json.dumps(data))
        response = requests.post(url, json.dumps(data), headers=headers)
        logging.info(
            "Got response code %s, content %s", response.status_code, response.content
        )
        preds = json.loads(response.content.decode("utf-8"))
        return preds


def explain(service_name, input_json):
    return explain_response(service_name, input_json)["data"]["precision"]


def explain_aix(service_name, input_json):
    return explain_response(service_name, input_json)["explanations"]["masks"][0]

def explain_art(service_name, input_json):
    return explain_response(service_name, input_json)["explanations"]["adversarial_prediction"]


def explain_response(service_name, input_json):
    isvc = KFServing.get(
        service_name,
        namespace=KFSERVING_TEST_NAMESPACE,
        version=constants.KFSERVING_V1BETA1_VERSION,
    )
    # temporary sleep until this is fixed https://github.com/kubeflow/kfserving/issues/604
    time.sleep(10)
    cluster_ip = get_cluster_ip()
    host = urlparse(isvc["status"]["url"]).netloc
    url = "http://{}/v1/models/{}:explain".format(cluster_ip, service_name)
    headers = {"Host": host}
    with open(input_json) as json_file:
        data = json.load(json_file)
        logging.info("Sending request data: %s", json.dumps(data))
        try:
            response = requests.post(url, json.dumps(data), headers=headers)
            logging.info(
                "Got response code %s, content %s",
                response.status_code,
                response.content,
            )
            json_response = json.loads(response.content.decode("utf-8"))
        except (RuntimeError, json.decoder.JSONDecodeError) as e:
            logging.info("Explain error -------")
            logging.info(
                KFServing.api_instance.get_namespaced_custom_object(
                    "serving.knative.dev",
                    "v1",
                    KFSERVING_TEST_NAMESPACE,
                    "services",
                    service_name + "-explainer",
                )
            )
            pods = KFServing.core_api.list_namespaced_pod(
                KFSERVING_TEST_NAMESPACE,
                label_selector="serving.kubeflow.org/inferenceservice={}".format(
                    service_name
                ),
            )
            for pod in pods.items:
                logging.info(pod)
                logging.info(
                    "%s\t%s\t%s"
                    % (pod.metadata.name, pod.status.phase, pod.status.pod_ip)
                )
                api_response = KFServing.core_api.read_namespaced_pod_log(
                    pod.metadata.name,
                    KFSERVING_TEST_NAMESPACE,
                    container="kfserving-container",
                )
                logging.info(api_response)
            raise e
        return json_response


def get_cluster_ip():
    api_instance = client.CoreV1Api(client.ApiClient())
    service = api_instance.read_namespaced_service(
        "istio-ingressgateway", "istio-system", exact="true"
    )
    if service.status.load_balancer.ingress is None:
        cluster_ip = service.spec.cluster_ip
    else:
        if service.status.load_balancer.ingress[0].hostname:
            cluster_ip = service.status.load_balancer.ingress[0].hostname
        else:
            cluster_ip = service.status.load_balancer.ingress[0].ip
    return os.environ.get("KFSERVING_INGRESS_HOST_PORT", cluster_ip)
