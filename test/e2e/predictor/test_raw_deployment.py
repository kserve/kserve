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

import os
import time
import requests
from kubernetes import client
from kfserving import (
    constants,
    KFServingClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
)
from kubernetes.client import V1Container, V1ContainerPort

from ..common.utils import get_cluster_ip
from ..common.utils import KFSERVING_TEST_NAMESPACE

api_version = constants.KFSERVING_V1BETA1

KFServing = KFServingClient(
    config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def test_raw_deployment_kfserving():
    containers = [V1Container(image='iamlovingit/hello:v2', name='hello',
                              ports=[V1ContainerPort(container_port=8080, protocol="TCP")])]
    service_name = "rawtest"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        containers=containers,
    )

    annotations = dict()
    annotations['serving.kubeflow.org/raw'] = 'true'
    annotations['kubernetes.io/ingress.class'] = 'istio'

    isvc = V1beta1InferenceService(
        api_version=api_version,
        kind=constants.KFSERVING_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KFSERVING_TEST_NAMESPACE, annotations=annotations,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    KFServing.create(isvc)
    KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)
    time.sleep(30)

    isvc = KFServing.get(
        service_name,
        namespace=KFSERVING_TEST_NAMESPACE,
    )

    cluster_ip = get_cluster_ip()

    host = isvc["status"]["url"]
    host = host[host.rfind('/')+1:]
    url = 'http://{}/hello'.format(cluster_ip)
    headers = {"Host": host}
    res = requests.get(url, headers=headers)
    assert(res.status_code == 200)

    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
