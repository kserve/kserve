# Copyright 2026 The KServe Authors.
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

"""
E2E test for independent prefill/decode direct-KEDA scaling.

Drives real traffic and asserts each role's ScaledObject reacts to its own
per-pod vllm:num_requests_running signal, filtered by pod name for prefill
vs. decode/main. Uses the per-pod gauge rather than the EPP request-rate
counter since there's one InferencePool/EPP per LLMInferenceService even in
disaggregated mode, so EPP metrics can't distinguish prefill from decode load.

Manually tagged with autoscaling_direct_keda (in addition to the
auto-discovered autoscaling_prefill_scaling marker) so it runs in the
existing direct-KEDA CI step without a dedicated CI job.
"""

import logging
import threading

import pytest

from .fixtures import generate_test_id, inject_k8s_proxy
from .logging import log_execution
from .test_llm_autoscaling_direct_keda import _cleanup, _new_kserve_client
from .test_llm_autoscaling_wva import (
    WORKLOAD_COMPONENT_MAIN,
    WORKLOAD_COMPONENT_PREFILL,
    assert_scaled_object_condition,
    send_load,
    wait_for_pod_count,
)
from .test_llm_inference_service import (
    TestCase,
    create_llmisvc,
    get_llm_service_url,
    wait_for_llm_isvc_ready,
)

logger = logging.getLogger(__name__)


def _create_and_wait(kserve_client, test_case):
    create_llmisvc(kserve_client, test_case.llm_service)
    wait_for_llm_isvc_ready(
        kserve_client, test_case.llm_service, test_case.wait_timeout
    )


@pytest.mark.autoscaling_direct_keda
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "prometheus-scrape-pd",
                    "workload-llmd-simulator-pd",
                    "scaling-decode-direct-keda",
                    "scaling-prefill-direct-keda",
                ],
                prompt="KServe is a",
                service_name="autoscale-prefill-direct-keda",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_prefill_decode_independent_scaling(test_case: TestCase):
    """P/D + direct KEDA: prefill and decode ScaledObjects react independently
    to their own per-pod vllm:num_requests_running signal under load."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name
    ns = test_case.namespace

    try:
        _create_and_wait(kserve_client, test_case)

        service_url = get_llm_service_url(kserve_client, test_case.llm_service)

        # vllm:num_requests_running is an instantaneous gauge with no
        # smoothing, so it decays right after load stops. Drive load from a
        # background thread so it's still in flight when we check Active/pod
        # counts below.
        load_thread = threading.Thread(
            target=send_load,
            args=(service_url, test_case.model_name),
            kwargs=dict(concurrency=10, duration_seconds=120, tolerate_failures=True),
            daemon=True,
        )
        load_thread.start()

        try:
            # Checked independently per role, before pod counts, while load
            # is still in flight (the gauge has no smoothing window).
            assert_scaled_object_condition(
                service_name,
                namespace=ns,
                condition_type="Active",
                expected_status="True",
                prefill=False,
                timeout=90,
            )
            assert_scaled_object_condition(
                service_name,
                namespace=ns,
                condition_type="Active",
                expected_status="True",
                prefill=True,
                timeout=90,
            )

            # Every request passes through both a prefill and a decode step,
            # so both roles scale beyond their minReplicas=1 baseline.
            wait_for_pod_count(
                service_name,
                min_count=2,
                namespace=ns,
                timeout=120,
                component=WORKLOAD_COMPONENT_MAIN,
            )
            wait_for_pod_count(
                service_name,
                min_count=2,
                namespace=ns,
                timeout=120,
                component=WORKLOAD_COMPONENT_PREFILL,
            )
        finally:
            load_thread.join(timeout=150)
    finally:
        _cleanup(kserve_client, test_case)
