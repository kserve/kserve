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
import json
import logging
import os
from urllib.parse import urlparse

import grpc
import portforward
import requests
from kubernetes import client

from kserve import KServeClient
from kserve import constants
from kserve.protocol.grpc import grpc_predict_v2_pb2 as pb
from kserve.protocol.grpc import grpc_predict_v2_pb2_grpc

from . import inference_pb2_grpc

logging.basicConfig(level=logging.INFO)

KSERVE_NAMESPACE = "kserve"
KSERVE_TEST_NAMESPACE = "kserve-ci-e2e-test"
MODEL_CLASS_NAME = "modelClass"
INFERENCESERVICE_CONTAINER = "kserve-container"
TRANSFORMER_CONTAINER = "transformer-container"
STORAGE_URI_ENV = "STORAGE_URI"


def grpc_stub(service_name, namespace):
    cluster_ip = get_cluster_ip()
    method_config = json.dumps({
        "methodConfig": [{
            "name": [{"service": "org.pytorch.serve.grpc.inference"}],
            "retryPolicy": {
                "maxAttempts": 5,
                "initialBackoff": "0.1s",
                "maxBackoff": "10s",
                "backoffMultiplier": 2,
                "retryableStatusCodes": ["UNAVAILABLE"],
            }}
        ]
    })
    os.environ["GRPC_VERBOSITY"] = "debug"
    channel = grpc.insecure_channel(
        cluster_ip,
        options=(('grpc.ssl_target_name_override', service_name + '.' + namespace + '.example.com'),
                 ('grpc.service_config', method_config)))
    return inference_pb2_grpc.InferenceAPIsServiceStub(channel)


def predict(service_name, input_json, protocol_version="v1",
            version=constants.KSERVE_V1BETA1_VERSION, model_name=None):
    with open(input_json) as json_file:
        data = json.load(json_file)

        return predict_str(service_name=service_name,
                           input_json=json.dumps(data),
                           protocol_version=protocol_version,
                           version=version,
                           model_name=model_name)


def predict_str(service_name, input_json, protocol_version="v1",
                version=constants.KSERVE_V1BETA1_VERSION, model_name=None):
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    cluster_ip, host, path = get_isvc_endpoint(isvc)
    headers = {"Host": host, "Content-Type": "application/json"}

    if model_name is None:
        model_name = service_name

    url = f"http://{cluster_ip}{path}/v1/models/{model_name}:predict"
    if protocol_version == "v2":
        url = f"http://{cluster_ip}{path}/v2/models/{model_name}/infer"

    logging.info("Sending Header = %s", headers)
    logging.info("Sending url = %s", url)
    logging.info("Sending request data: %s", input_json)
    response = requests.post(url, input_json, headers=headers)
    logging.info("Got response code %s, content %s", response.status_code, response.content)
    if response.status_code == 200:
        preds = json.loads(response.content.decode("utf-8"))
        return preds
    else:
        response.raise_for_status()


def predict_ig(ig_name, input_json, protocol_version="v1",
               version=constants.KSERVE_V1ALPHA1_VERSION):
    with open(input_json) as json_file:
        data = json.dumps(json.load(json_file))

        kserve_client = KServeClient(
            config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
        ig = kserve_client.get_inference_graph(
            ig_name,
            namespace=KSERVE_TEST_NAMESPACE,
            version=version,
        )

        cluster_ip, host, _ = get_isvc_endpoint(ig)
        headers = {"Host": host}
        url = f"http://{cluster_ip}"

        logging.info("Sending Header = %s", headers)
        logging.info("Sending url = %s", url)
        logging.info("Sending request data: %s", input_json)
        response = requests.post(url, data, headers=headers)
        logging.info("Got response code %s, content %s", response.status_code, response.content)
        if response.status_code == 200:
            preds = json.loads(response.content.decode("utf-8"))
            return preds
        else:
            response.raise_for_status()


def explain(service_name, input_json):
    return explain_response(service_name, input_json)["data"]["precision"]


def explain_art(service_name, input_json):
    return explain_response(
        service_name, input_json)["explanations"]["adversarial_prediction"]


def explain_response(service_name, input_json):
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=constants.KSERVE_V1BETA1_VERSION,
    )
    cluster_ip, host, _ = get_isvc_endpoint(isvc)
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
            if response.status_code == 200:
                json_response = json.loads(response.content.decode("utf-8"))
            else:
                response.raise_for_status()
        except (RuntimeError, json.decoder.JSONDecodeError) as e:
            logging.info("Explain error -------")
            pods = kfs_client.core_api.list_namespaced_pod(
                KSERVE_TEST_NAMESPACE,
                label_selector="serving.kserve.io/inferenceservice={}".format(
                    service_name),
            )
            for pod in pods.items:
                logging.info(pod)
                logging.info(
                    "%s\t%s\t%s" %
                    (pod.metadata.name, pod.status.phase, pod.status.pod_ip))
                api_response = kfs_client.core_api.read_namespaced_pod_log(
                    pod.metadata.name,
                    KSERVE_TEST_NAMESPACE,
                    container="kserve-container",
                )
                logging.info(api_response)
            raise e
        return json_response


def get_cluster_ip(name="istio-ingressgateway", namespace="istio-system"):
    cluster_ip = os.environ.get("KSERVE_INGRESS_HOST_PORT")
    if cluster_ip is None:
        api_instance = client.CoreV1Api(client.ApiClient())
        service = api_instance.read_namespaced_service(name, namespace)
        if service.status.load_balancer.ingress is None:
            cluster_ip = service.spec.cluster_ip
        else:
            if service.status.load_balancer.ingress[0].hostname:
                cluster_ip = service.status.load_balancer.ingress[0].hostname
            else:
                cluster_ip = service.status.load_balancer.ingress[0].ip
    return cluster_ip


def predict_grpc(service_name, payload, parameters=None, version=constants.KSERVE_V1BETA1_VERSION, model_name=None):
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    cluster_ip, host, _ = get_isvc_endpoint(isvc)
    if ":" not in cluster_ip:
        cluster_ip = cluster_ip + ":80"

    if model_name is None:
        model_name = service_name
    logging.info("Cluster IP: %s", cluster_ip)
    logging.info("gRPC target host: %s", host)

    channel = grpc.insecure_channel(
        cluster_ip,
        options=(('grpc.ssl_target_name_override', host),))
    stub = grpc_predict_v2_pb2_grpc.GRPCInferenceServiceStub(channel)
    return stub.ModelInfer(pb.ModelInferRequest(model_name=model_name, inputs=payload, parameters=parameters))


def predict_modelmesh(service_name, input_json, pod_name, model_name=None):
    with open(input_json) as json_file:
        data = json.load(json_file)

        if model_name is None:
            model_name = service_name
        with portforward.forward("default", pod_name, 8008, 8008, waiting=5):
            url = f"http://localhost:8008/v2/models/{model_name}/infer"
            response = requests.post(url, json.dumps(data))
            return json.loads(response.content.decode("utf-8"))


def get_isvc_endpoint(isvc):
    host = urlparse(isvc["status"]["url"]).netloc
    path = urlparse(isvc["status"]["url"]).path
    if os.environ.get("CI_USE_ISVC_HOST") == "1":
        cluster_ip = host
    else:
        cluster_ip = get_cluster_ip()
    return cluster_ip, host, path
