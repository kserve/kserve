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

Predictor tests follow the pattern ``create -> wait_isvc_ready -> assert ->
delete``. If ``wait_isvc_ready`` raises (slow cold start, scheduling failure,
etc.) the trailing ``delete`` is never reached and the InferenceService is
leaked into the cluster, where its Deployment keeps a Pod holding CPU/memory
requests and blocks subsequent tests on resource-constrained CI runners.

Additionally, ``KServeClient.delete`` is asynchronous: even on the happy path
the pod stays in its graceful-termination window while the next test's
``create`` may already be racing for the same capacity.

This autouse fixture, after each test:
  1. Force-deletes any InferenceService the test created (tracked by
     monkey-patching ``KServeClient.create`` for the duration of the test).
     Idempotent: already-deleted ISVCs are ignored.
  2. Polls until any predictor pods carrying a ``deletionTimestamp`` in the
     test namespace have fully disappeared, so the next test's scheduling
     decision sees freed capacity.

Pytest-xdist runs each worker in its own process, so the tracking state is
worker-local and the cleanup only touches ISVCs this worker created — peers
running concurrent tests are unaffected.
"""

import os
import time

from kserve import KServeClient
from kubernetes import client as k8s_client
from kubernetes import config as k8s_config
import pytest

from ..common.utils import KSERVE_TEST_NAMESPACE

_ISVC_POD_LABEL = "serving.kserve.io/inferenceservice"
_TERMINATION_WAIT_TIMEOUT_S = 60
_TERMINATION_POLL_INTERVAL_S = 2
# Brief grace before the first poll so we don't race the controller's
# propagation of a just-issued ``delete()`` (ISVC -> Knative -> Deployment
# scale-to-0 -> pod ``deletionTimestamp``) — usually 1-2s in practice.
_TERMINATION_PROPAGATION_GRACE_S = 5


@pytest.fixture(autouse=True)
def _cleanup_orphaned_predictor_isvcs(monkeypatch):
    """Track ISVCs created during the test and force-delete any survivors.

    Never raises: a failure in cleanup must not mask the real test result.
    """
    created: list[tuple[str, str]] = []
    original_create = KServeClient.create

    def _tracking_create(self, inferenceservice, *args, **kwargs):
        result = original_create(self, inferenceservice, *args, **kwargs)
        try:
            meta = inferenceservice.metadata
            name = meta.name
            namespace = meta.namespace or KSERVE_TEST_NAMESPACE
            if name:
                created.append((name, namespace))
        except Exception:
            pass
        return result

    monkeypatch.setattr(KServeClient, "create", _tracking_create)

    yield

    if created:
        try:
            cleanup_client = KServeClient(
                config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
            )
            for name, namespace in created:
                try:
                    cleanup_client.delete(name, namespace)
                except Exception:
                    pass
        except Exception:
            pass

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
