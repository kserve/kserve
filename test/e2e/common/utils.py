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
import os
import time
from concurrent import futures
from typing import Union, List, Dict
from urllib.parse import urlparse

import portforward
import requests
from kubernetes import client as k8s_client
from orjson import orjson

from kserve import KServeClient, InferResponse, InferRequest
from kserve import constants
from kserve.inference_client import InferenceGRPCClient, InferenceRESTClient
from kserve.protocol.grpc import grpc_predict_v2_pb2 as pb
from kserve.logging import trace_logger as logger

KSERVE_NAMESPACE = "kserve"
KSERVE_TEST_NAMESPACE = "kserve-ci-e2e-test"
MODEL_CLASS_NAME = "modelClass"
INFERENCESERVICE_CONTAINER = "kserve-container"
TRANSFORMER_CONTAINER = "transformer-container"
STORAGE_URI_ENV = "STORAGE_URI"


def grpc_client(host, cluster_ip):
    if ":" not in cluster_ip:
        cluster_ip = cluster_ip + ":80"
    logger.info("Cluster IP: %s", cluster_ip)
    logger.info("gRPC target host: %s", host)
    return InferenceGRPCClient(
        cluster_ip,
        verbose=True,
        channel_args=[
            ("grpc.ssl_target_name_override", host),
        ],
        timeout=120,
    )


async def predict_isvc(
    client: InferenceRESTClient,
    service_name,
    input: Union[str, InferRequest],
    version=constants.KSERVE_V1BETA1_VERSION,
    model_name=None,
    is_batch=False,
    network_layer: str = "istio",
) -> Union[InferResponse, Dict, List[Union[Dict, InferResponse]]]:
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc, network_layer)
    if model_name is None:
        model_name = service_name
    base_url = f"{scheme}://{cluster_ip}{path}"
    return await predict(
        client,
        base_url,
        host,
        input,
        model_name=model_name,
        is_batch=is_batch,
        is_graph=False,
    )


async def predict(
    client: InferenceRESTClient,
    url,
    host,
    input: Union[str, InferRequest],
    model_name=None,
    is_batch=False,
    is_graph=False,
) -> Union[InferResponse, Dict, List[Union[Dict, InferResponse]]]:
    if isinstance(input, str):
        with open(input) as json_file:
            data = json.load(json_file)
    else:
        data = input
    headers = {"Host": host, "Content-Type": "application/json"}
    if is_batch:
        with futures.ThreadPoolExecutor(max_workers=4) as executor:
            loop = asyncio.get_event_loop()
            future_list = [
                await loop.run_in_executor(
                    executor,
                    _predict,
                    client,
                    url,
                    input_data,
                    model_name,
                    headers,
                    is_graph,
                )
                for input_data in data
            ]
            result = await asyncio.gather(*future_list)
    else:
        result = await _predict(
            client,
            url,
            data,
            model_name=model_name,
            headers=headers,
            is_graph=is_graph,
        )
    logger.info("Got response %s", result)
    return result


async def _predict(
    client: InferenceRESTClient,
    url,
    input_data,
    model_name,
    headers=None,
    is_graph=False,
) -> Union[InferResponse, Dict]:
    logger.info("Sending Header = %s", headers)
    logger.info("base url = %s", url)
    # temporary sleep until this is fixed https://github.com/kserve/kserve/issues/604
    await asyncio.sleep(5)
    response = await client.infer(
        url,
        input_data,
        model_name=model_name,
        headers=headers,
        is_graph_endpoint=is_graph,
    )
    return response


async def predict_ig(
    client: InferenceRESTClient,
    ig_name,
    input_path,
    version=constants.KSERVE_V1ALPHA1_VERSION,
    network_layer: str = "istio",
) -> Union[InferResponse, Dict]:
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    ig = kserve_client.get_inference_graph(
        ig_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    scheme, cluster_ip, host, _ = get_isvc_endpoint(ig, network_layer)
    url = f"{scheme}://{cluster_ip}"
    return await predict(client, url, host, input_path, is_graph=True)


async def explain_art(
    client, service_name, input_path, network_layer: str = "istio"
) -> Dict:
    res = await explain_response(
        client, service_name, input_path, network_layer=network_layer
    )
    return res["explanations"]["adversarial_prediction"]


async def explain_response(
    client, service_name, input_path, network_layer: str
) -> Dict:
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=constants.KSERVE_V1BETA1_VERSION,
    )
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc, network_layer)
    url = f"{scheme}://{cluster_ip}{path}"
    headers = {"Host": host}
    with open(input_path) as json_file:
        data = json.load(json_file)
        logger.info("Sending request data: %s", data)
        try:
            # temporary sleep until this is fixed https://github.com/kserve/kserve/issues/604
            await asyncio.sleep(5)
            response = await client.explain(
                url, model_name=service_name, data=data, headers=headers
            )
        except (RuntimeError, orjson.JSONDecodeError) as e:
            logger.info("Explain error -------")
            pods = kfs_client.core_api.list_namespaced_pod(
                KSERVE_TEST_NAMESPACE,
                label_selector="serving.kserve.io/inferenceservice={}".format(
                    service_name
                ),
            )
            for pod in pods.items:
                logger.info(pod)
                logger.info(
                    "%s\t%s\t%s"
                    % (pod.metadata.name, pod.status.phase, pod.status.pod_ip)
                )
                api_response = kfs_client.core_api.read_namespaced_pod_log(
                    pod.metadata.name,
                    KSERVE_TEST_NAMESPACE,
                    container="kserve-container",
                )
                logger.info(api_response)
            raise e
        return response


def get_cluster_ip(namespace="istio-system", labels: dict = None):
    cluster_ip = os.environ.get("KSERVE_INGRESS_HOST_PORT")
    if cluster_ip is None:
        api_instance = k8s_client.CoreV1Api(k8s_client.ApiClient())
        if labels is None:
            labels = {
                "app": "istio-ingressgateway",
                "istio": "ingressgateway",
            }
        label_selector = ",".join([f"{key}={value}" for key, value in labels.items()])
        services = api_instance.list_namespaced_service(
            namespace, label_selector=label_selector
        )
        if services.items:
            service = services.items[0]
        else:
            raise RuntimeError(f"No service found with labels: {labels}")

        if service.status.load_balancer.ingress is None:
            cluster_ip = service.spec.cluster_ip
        else:
            if service.status.load_balancer.ingress[0].hostname:
                cluster_ip = service.status.load_balancer.ingress[0].hostname
            else:
                cluster_ip = service.status.load_balancer.ingress[0].ip
    return cluster_ip


async def predict_grpc(
    service_name,
    payload,
    parameters=None,
    version=constants.KSERVE_V1BETA1_VERSION,
    model_name=None,
    network_layer: str = "istio",
) -> InferResponse:
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    _, cluster_ip, host, _ = get_isvc_endpoint(isvc, network_layer)

    if model_name is None:
        model_name = service_name
    client = grpc_client(host, cluster_ip)

    response = await client.infer(
        InferRequest.from_grpc(
            pb.ModelInferRequest(
                model_name=model_name, inputs=payload, parameters=parameters
            )
        )
    )
    return response


async def predict_modelmesh(
    client, service_name, input_json, pod_name, model_name=None
) -> InferResponse:
    with open(input_json) as json_file:
        data = json.load(json_file)

    if model_name is None:
        model_name = service_name
    with portforward.forward("default", pod_name, 8008, 8008, waiting=5):
        headers = {"Content-Type": "application/json"}
        url = "http://localhost:8008"
        logger.info("Sending Header = %s", headers)
        logger.info("base url = %s", url)

        response = await client.infer(url, data, model_name=model_name, headers=headers)
        logger.info("Got response content %s", response)
        return response


def get_isvc_endpoint(isvc, network_layer: str = "istio"):
    scheme = urlparse(isvc["status"]["url"]).scheme
    host = urlparse(isvc["status"]["url"]).netloc
    path = urlparse(isvc["status"]["url"]).path
    if os.environ.get("CI_USE_ISVC_HOST") == "1":
        cluster_ip = host
    elif network_layer == "istio" or network_layer == "istio-ingress":
        cluster_ip = get_cluster_ip()
    elif network_layer == "envoy-gatewayapi":
        cluster_ip = get_cluster_ip(
            namespace="envoy-gateway-system",
            labels={"serving.kserve.io/gateway": "kserve-ingress-gateway"},
        )
    elif network_layer == "istio-gatewayapi":
        cluster_ip = get_cluster_ip(
            namespace="kserve",
            labels={"serving.kserve.io/gateway": "kserve-ingress-gateway"},
        )
    else:
        raise ValueError(f"Unknown network layer {network_layer}")
    return scheme, cluster_ip, host, path


def generate(
    service_name,
    input_json,
    version=constants.KSERVE_V1BETA1_VERSION,
    chat_completions=True,
):
    url_suffix = "v1/chat/completions" if chat_completions else "v1/completions"
    return _openai_request(service_name, input_json, version, url_suffix)


def embed(
    service_name,
    input_json,
    version=constants.KSERVE_V1BETA1_VERSION,
):
    return _openai_request(service_name, input_json, version, "v1/embeddings")


def _get_openai_endpoint_and_host(service_name, url_suffix):
    """
    Get the OpenAI endpoint for the given service name.

    Args:
        service_name: The name of the inference service
        url_suffix: The suffix for the OpenAI endpoint (e.g., "v1/chat/completions")

    Returns:
        A tuple containing the OpenAI endpoint URL and the host name
    """
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=constants.KSERVE_V1BETA1_VERSION,
    )
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc)
    return f"{scheme}://{cluster_ip}{path}/openai/{url_suffix}", host


def transcribe(service_name, audio_file_path, stream=False, model_name=None):
    """
    Send a transcription request to the inference service with an audio file.

    Args:
        service_name: The name of the inference service
        audio_file_path: Path to the audio file to transcribe
        stream: Whether to stream the response
        model_name: The name of the model to use for transcription

    Returns:
        JSON response from the transcription service if stream is False
        Otherwise, returns the transcript as a string
    """
    if model_name is None:
        model_name = service_name
    url, host = _get_openai_endpoint_and_host(service_name, "v1/audio/transcriptions")
    with open(audio_file_path, "rb") as f:
        file_content = f.read()
    headers = {"Host": host}

    logger.info("Sending Headers = %s", headers)
    logger.info("Sending url = %s", url)
    # temporary sleep until this is fixed https://github.com/kserve/kserve/issues/604
    time.sleep(10)

    response = requests.post(
        url,
        files={"file": file_content},
        data={"stream": stream, "model": model_name},
        stream=stream,
        headers=headers,
    )
    logger.info("Got response code %s", response.status_code)
    if response.status_code == 200:
        if stream:
            transcript = ""
            for line in response.iter_lines():
                if line:
                    decoded_line = line.decode("utf-8").strip()
                    if decoded_line == "data: [DONE]":
                        break  # end of stream
                    json_str = decoded_line.replace("data: ", "", 1)
                    payload = json.loads(json_str)
                    for choice in payload.get("choices", []):
                        content = choice.get("delta", {}).get("content")
                        if content:
                            transcript += content

            return transcript
        transcript = json.loads(response.content.decode("utf-8"))
        return transcript
    else:
        response.raise_for_status()


def _openai_request(
    service_name,
    input_json,
    version=constants.KSERVE_V1BETA1_VERSION,
    url_suffix="",
):
    with open(input_json) as json_file:
        data = json.load(json_file)

        url, host = _get_openai_endpoint_and_host(service_name, url_suffix)
        headers = {"Host": host, "Content-Type": "application/json"}

        logger.info("Sending Header = %s", headers)
        logger.info("Sending url = %s", url)
        logger.info("Sending request data: %s", data)
        # temporary sleep until this is fixed https://github.com/kserve/kserve/issues/604
        time.sleep(10)
        response = requests.post(url, json.dumps(data), headers=headers)
        logger.info(
            "Got response code %s, content %s", response.status_code, response.content
        )
        if response.status_code == 200:
            preds = json.loads(response.content.decode("utf-8"))
            return preds
        else:
            response.raise_for_status()


def is_model_ready(
    rest_client,
    service_name,
    model_name,
    version=constants.KSERVE_V1BETA1_VERSION,
    network_layer: str = "istio",
):
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc, network_layer)
    if model_name is None:
        model_name = service_name
    base_url = f"{scheme}://{cluster_ip}{path}"
    headers = {"Host": host}
    return rest_client.is_model_ready(base_url, model_name, headers=headers)


def extract_process_ids_from_logs(logs: str) -> set[int]:
    process_ids = set()
    for line in logs.splitlines():
        tokens = line.strip().split()
        if len(tokens) >= 5 and tokens[3] == "kserve.trace":
            process_ids.add(int(tokens[2]))
    logger.info("Extracted process ids: %s", process_ids)
    return process_ids
