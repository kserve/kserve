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

"""Namespace provisioning checks without a cluster: python -m unittest test.test_e2e_namespace."""

import copy
import unittest
from unittest.mock import Mock, patch
from uuid import UUID

from kubernetes import client

from test.e2e.common import namespace


class NamespaceProvisioningTest(unittest.TestCase):
    def test_consecutive_sessions_do_not_reuse_a_terminating_namespace(self):
        with patch.object(
            namespace,
            "uuid4",
            side_effect=[
                UUID("11111111-1111-4111-8111-111111111111"),
                UUID("22222222-2222-4222-8222-222222222222"),
            ],
            create=True,
        ):
            previous = namespace.worker_namespace_name("gw0")
            current = namespace.worker_namespace_name("gw0")
        self.assertNotEqual(previous, current)
        self.assertLessEqual(len(current), 18)
        self.assertRegex(current, r"^e2e-gw0-[a-f0-9]+$")

    def test_worker_namespace_fits_component_hostnames(self):
        # Component names from the raw collocation and gRPC transformer tests.
        for worker_id in ("gw0", "master", "gw1234567890"):
            worker_namespace = namespace.worker_namespace_name(worker_id)
            for component in (
                "raw-custom-model-collocation-12345-predictor",
                "model-grpc-trans-grpc-raw-12345-transformer",
            ):
                for hostname in (
                    f"{component}-{worker_namespace}.example.com",
                    f"{component}.{worker_namespace}.example.com",
                ):
                    with self.subTest(hostname=hostname):
                        for label in hostname.split("."):
                            self.assertLessEqual(len(label), 63)
                            self.assertRegex(label, r"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")

    def test_namespaced_runtime_is_copied_without_server_metadata(self):
        runtime = {
            "apiVersion": "serving.kserve.io/v1alpha1",
            "kind": "ServingRuntime",
            "metadata": {
                "name": "custom-runtime",
                "namespace": "seed",
                "uid": "source-uid",
                "resourceVersion": "42",
                "ownerReferences": [{"uid": "source-owner"}],
                "finalizers": ["source-finalizer"],
                "labels": {"app": "runtime"},
                "annotations": {
                    "runtime.example/setting": "value",
                    "kubectl.kubernetes.io/last-applied-configuration": "seed object",
                },
            },
            "spec": {
                "supportedModelFormats": [{"name": "sklearn", "autoSelect": True}],
                "containers": [{"name": "kserve-container", "image": "custom/image"}],
            },
            "status": {"conditions": []},
        }
        original = copy.deepcopy(runtime)
        api = Mock()
        api.list_namespaced_custom_object.return_value = {"items": [runtime]}

        namespace.provision_serving_runtimes(api, "worker")

        api.list_namespaced_custom_object.assert_called_once_with(
            "serving.kserve.io", "v1alpha1", namespace.SEED_NAMESPACE, "servingruntimes"
        )
        body = api.create_namespaced_custom_object.call_args.args[-1]
        self.assertEqual(body["spec"], original["spec"])
        self.assertEqual(
            body["metadata"],
            {
                "name": "custom-runtime",
                "namespace": "worker",
                "labels": {"app": "runtime"},
                "annotations": {"runtime.example/setting": "value"},
            },
        )
        self.assertNotIn("status", body)
        self.assertEqual(runtime, original)
        api.list_cluster_custom_object.assert_not_called()

        # Existing worker runtimes are left alone, but admission/RBAC errors fail setup.
        for status in (409, 403, 500):
            with self.subTest(status=status):
                api.create_namespaced_custom_object.side_effect = (
                    client.rest.ApiException(status=status)
                )
                if status == 409:
                    namespace.provision_serving_runtimes(api, "worker")
                else:
                    with self.assertRaises(client.rest.ApiException):
                        namespace.provision_serving_runtimes(api, "worker")

    def test_cluster_runtime_only_setup_needs_no_copies(self):
        api = Mock()
        api.list_namespaced_custom_object.return_value = {"items": []}
        namespace.provision_serving_runtimes(api, "worker")
        api.create_namespaced_custom_object.assert_not_called()
        api.list_cluster_custom_object.assert_not_called()

    def test_missing_seed_does_not_break_cluster_runtime_setup(self):
        api = Mock()
        api.list_namespaced_custom_object.side_effect = client.rest.ApiException(
            status=404
        )
        namespace.provision_serving_runtimes(api, "worker")
        api.create_namespaced_custom_object.assert_not_called()

    def test_runtime_api_errors_are_not_hidden(self):
        api = Mock()
        api.list_namespaced_custom_object.side_effect = client.rest.ApiException(
            status=403
        )
        with self.assertRaises(client.rest.ApiException):
            namespace.provision_serving_runtimes(api, "worker")

    def test_revision_based_istio_injection_is_preserved(self):
        core = Mock()
        core.read_namespace.return_value = client.V1Namespace(
            metadata=client.V1ObjectMeta(
                labels={
                    "istio.io/rev": "stable",
                    "pod-security.kubernetes.io/enforce": "restricted",
                    "kubernetes.io/metadata.name": "seed",
                }
            )
        )
        self.assertEqual(
            namespace._namespace_labels(core),
            {
                "kserve.io/e2e-test": "true",
                "istio.io/rev": "stable",
                "pod-security.kubernetes.io/enforce": "restricted",
            },
        )


if __name__ == "__main__":
    unittest.main()
