# Copyright 2024 The KServe Authors.
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

import asyncio
import json
import logging
import logging.config
import os
from concurrent import futures
from typing import Union, List, Dict
from urllib.parse import urlparse

import portforward
from kubernetes import client as k8s_client
from orjson import orjson

from kserve import KServeClient, InferResponse
from kserve import constants
from kserve.inference_client import InferenceRESTClient, InferenceGRPCClient, RESTConfig
from kserve.model import PredictorProtocol
from kserve.protocol.grpc import grpc_predict_v2_pb2 as pb
from kserve.protocol.grpc import grpc_predict_v2_pb2_grpc

from . import inference_pb2_grpc

from kserve.protocol.grpc.grpc_predict_v2_pb2 import ModelInferResponse

KSERVE_NAMESPACE = "kserve"
KSERVE_TEST_NAMESPACE = "kserve-ci-e2e-test"
MODEL_CLASS_NAME = "modelClass"
INFERENCESERVICE_CONTAINER = "kserve-container"
TRANSFORMER_CONTAINER = "transformer-container"
STORAGE_URI_ENV = "STORAGE_URI"

rest_client_v1 = None
rest_client_v2 = None
grpc_client = None

logging.basicConfig(level=logging.INFO)
LOG_CONFIG = {
    "version": 1,
    "disable_existing_loggers": False,
    "handlers": {
        "kserve": {
            "class": "logging.StreamHandler",
            "stream": "ext://sys.stderr",
        },
    },
    "loggers": {
        "kserve": {"handlers": ["kserve"], "level": "INFO", "propagate": True},
    },
}
logging.config.dictConfig(LOG_CONFIG)


def grpc_stub(host):
    cluster_ip = get_cluster_ip()
    if ":" not in cluster_ip:
        cluster_ip = cluster_ip + ":80"
    logging.info("Cluster IP: %s", cluster_ip)
    logging.info("gRPC target host: %s", host)
    os.environ["GRPC_VERBOSITY"] = "debug"
    if grpc_client is None:
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
        return InferenceGRPCClient(cluster_ip, verbose=True, channel_args=[
            ('grpc.ssl_target_name_override', host),
            ('grpc.service_config', method_config),
        ])
    return grpc_client


def get_rest_client(protocol):
    global rest_client_v1
    global rest_client_v2
    if protocol == PredictorProtocol.REST_V1.value:
        if rest_client_v1 is None:
            rest_client_v1 = InferenceRESTClient(config=RESTConfig(timeout=10, verbose=True, protocol=protocol))
        return rest_client_v1
    else:
        if rest_client_v2 is None:
            rest_client_v2 = InferenceRESTClient(config=RESTConfig(timeout=10, verbose=True, protocol=protocol))
        return rest_client_v2


async def predict_isvc(service_name, input_path, protocol_version="v1",
                       version=constants.KSERVE_V1BETA1_VERSION, model_name=None, is_batch=False) \
        -> Union[InferResponse, Dict, List[Union[Dict, InferResponse]]]:
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    cluster_ip, host, path = get_isvc_endpoint(isvc)
    if model_name is None:
        model_name = service_name
    base_url = f"http://{cluster_ip}{path}"
    return await predict(base_url, host, input_path, protocol_version, is_batch)


async def predict(url, host, input_path, protocol_version="v1", is_batch=False) \
        -> Union[InferResponse, Dict, List[Union[Dict, InferResponse]]]:
    with open(input_path) as json_file:
        data = json.load(json_file)
    headers = {"Host": host, "Content-Type": "application/json"}
    if is_batch:
        with futures.ThreadPoolExecutor(max_workers=4) as executor:
            loop = asyncio.get_event_loop()
            future_list = [await loop.run_in_executor(executor, _predict, url, input_data, headers,
                                                      protocol_version) for input_data in data]
            result = await asyncio.gather(*future_list)
    else:
        result = await _predict(url, data, headers, protocol_version)
    logging.info("Got response %s", result)
    return result


async def _predict(url, input_data, headers=None, protocol_version="v1") -> Union[InferResponse, Dict]:
    client = get_rest_client(protocol=protocol_version)
    logging.info("Sending Header = %s", headers)
    logging.info("Sending url = %s", url)
    logging.info("Sending request data: %s", input_data)
    # temporary sleep until this is fixed https://github.com/kserve/kserve/issues/604
    await asyncio.sleep(3)
    response = await client.infer(url, input_data, headers)
    return response


async def predict_ig(ig_name, input_path, protocol_version="v1",
                     version=constants.KSERVE_V1ALPHA1_VERSION) -> Union[InferResponse, Dict]:
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    ig = kserve_client.get_inference_graph(
        ig_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    cluster_ip, host, _ = get_isvc_endpoint(ig)
    url = f"http://{cluster_ip}"
    return await predict(url, host, input_path, protocol_version)


async def explain(service_name, input_path):
    res = await explain_response(service_name, input_path)
    return res["data"]["precision"]


async def explain_art(service_name, input_path):
    res = await explain_response(service_name, input_path)
    return res["explanations"][
        "adversarial_prediction"
    ]


async def explain_response(service_name, input_path) -> Dict:
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=constants.KSERVE_V1BETA1_VERSION,
    )
    cluster_ip, host, _ = get_isvc_endpoint(isvc)
    url = "http://{}/v1/models/{}:explain".format(cluster_ip, service_name)
    headers = {"Host": host}
    with open(input_path) as json_file:
        data = json.load(json_file)
        logging.info("Sending request data: %s", data)
        try:
            # temporary sleep until this is fixed https://github.com/kserve/kserve/issues/604
            await asyncio.sleep(3)
            client = get_rest_client()
            response = await client.explain(url, data, headers=headers)
        except (RuntimeError, orjson.JSONDecodeError) as e:
            logging.info("Explain error -------")
            pods = kfs_client.core_api.list_namespaced_pod(
                KSERVE_TEST_NAMESPACE,
                label_selector="serving.kserve.io/inferenceservice={}".format(
                    service_name
                ),
            )
            for pod in pods.items:
                logging.info(pod)
                logging.info(
                    "%s\t%s\t%s"
                    % (pod.metadata.name, pod.status.phase, pod.status.pod_ip)
                )
                api_response = kfs_client.core_api.read_namespaced_pod_log(
                    pod.metadata.name,
                    KSERVE_TEST_NAMESPACE,
                    container="kserve-container",
                )
                logging.info(api_response)
            raise e
        return response


def get_cluster_ip(name="istio-ingressgateway", namespace="istio-system"):
    cluster_ip = os.environ.get("KSERVE_INGRESS_HOST_PORT")
    if cluster_ip is None:
        api_instance = k8s_client.CoreV1Api(k8s_client.ApiClient())
        service = api_instance.read_namespaced_service(name, namespace)
        if service.status.load_balancer.ingress is None:
            cluster_ip = service.spec.cluster_ip
        else:
            if service.status.load_balancer.ingress[0].hostname:
                cluster_ip = service.status.load_balancer.ingress[0].hostname
            else:
                cluster_ip = service.status.load_balancer.ingress[0].ip
    return cluster_ip


async def predict_grpc(service_name, payload, parameters=None, version=constants.KSERVE_V1BETA1_VERSION,
                       model_name=None,) -> ModelInferResponse:
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    _, host, _ = get_isvc_endpoint(isvc)

    if model_name is None:
        model_name = service_name
    client = grpc_stub(host)
    response = await client.infer(pb.ModelInferRequest(model_name=model_name, inputs=payload, parameters=parameters))
    return response


async def predict_modelmesh(service_name, input_json, pod_name, model_name=None) -> InferResponse:
    with open(input_json) as json_file:
        data = json.load(json_file)

    if model_name is None:
        model_name = service_name
    with portforward.forward("default", pod_name, 8008, 8008, waiting=5):
        headers = {"Content-Type": "application/json"}
        url = f"http://localhost:8008/v2/models/{model_name}/infer"
        logging.info("Sending Header = %s", headers)
        logging.info("Sending url = %s", url)
        logging.info("Sending request data: %s", data)

        client = get_rest_client()
        response = await client.infer(url, data, headers)
        logging.info("Got response content %s", response)
        return response


def get_isvc_endpoint(isvc):
    host = urlparse(isvc["status"]["url"]).netloc
    path = urlparse(isvc["status"]["url"]).path
    if os.environ.get("CI_USE_ISVC_HOST") == "1":
        cluster_ip = host
    else:
        cluster_ip = get_cluster_ip()
    return cluster_ip, host, path


def generate(
    service_name,
    input_json,
    version=constants.KSERVE_V1BETA1_VERSION,
    chat_completions=True,
):
    with open(input_json) as json_file:
        data = json.load(json_file)

        kfs_client = KServeClient(
            config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
        )
        isvc = kfs_client.get(
            service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            version=version,
        )
        cluster_ip, host, path = get_isvc_endpoint(isvc)
        headers = {"Host": host, "Content-Type": "application/json"}

        if chat_completions:
            url = f"http://{cluster_ip}{path}/openai/v1/chat/completions"
        else:
            url = f"http://{cluster_ip}{path}/openai/v1/completions"
        logging.info("Sending Header = %s", headers)
        logging.info("Sending url = %s", url)
        logging.info("Sending request data: %s", data)
        # temporary sleep until this is fixed https://github.com/kserve/kserve/issues/604
        time.sleep(10)
        response = requests.post(url, json.dumps(data), headers=headers)
        logging.info(
            "Got response code %s, content %s", response.status_code, response.content
        )
        if response.status_code == 200:
            preds = json.loads(response.content.decode("utf-8"))
            return preds
        else:
            response.raise_for_status()
