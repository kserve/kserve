# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Predictor-scope pytest fixtures.

``KServeClient.delete()`` is asynchronous: it returns as soon as the API
server accepts the request, leaving the predictor pod to drain in the
background while still holding its CPU/memory requests. Back-to-back tests
can therefore contend for cluster capacity even though the previous test
has logically completed.

This autouse fixture, after each test, polls until any predictor pods that
are *actively terminating* in the test namespace have fully disappeared.
We only wait on pods carrying a ``deletionTimestamp`` so we don't block on
peers that are still mid-test under parallel pytest workers.
"""

import time

from kubernetes import client as k8s_client
from kubernetes import config as k8s_config
import pytest

from ..common.utils import KSERVE_TEST_NAMESPACE

_ISVC_POD_LABEL = "serving.kserve.io/inferenceservice"
_TERMINATION_WAIT_TIMEOUT_S = 180
_TERMINATION_POLL_INTERVAL_S = 2
# Brief grace before the first poll so we don't race the controller's
# propagation of a just-issued ``delete()`` (ISVC -> Knative -> Deployment
# scale-to-0 -> pod ``deletionTimestamp``) — usually 1-2s in practice.
_TERMINATION_PROPAGATION_GRACE_S = 5


@pytest.fixture(autouse=True)
def _wait_for_terminating_predictor_pods():
    """Block the next test until predictor pods marked for deletion are gone.

    Bounded by ``_TERMINATION_WAIT_TIMEOUT_S``; never raises on timeout so a
    slow teardown can't mask the real test result.
    """
    yield

    try:
        try:
            k8s_config.load_incluster_config()
        except k8s_config.ConfigException:
            k8s_config.load_kube_config()
    except Exception:
        return

    api = k8s_client.CoreV1Api()
    time.sleep(_TERMINATION_PROPAGATION_GRACE_S)
    deadline = time.monotonic() + _TERMINATION_WAIT_TIMEOUT_S
    while time.monotonic() < deadline:
        try:
            pods = api.list_namespaced_pod(
                namespace=KSERVE_TEST_NAMESPACE,
                label_selector=_ISVC_POD_LABEL,
            ).items
        except k8s_client.rest.ApiException:
            return

        terminating = [p for p in pods if p.metadata.deletion_timestamp is not None]
        if not terminating:
            return
        time.sleep(_TERMINATION_POLL_INTERVAL_S)
