# Copyright 2025 The KServe Authors.
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

GATEWAY_CLASS_NAME = os.environ.get("GATEWAY_CLASS_NAME", "envoy")


def make_router_gateway(name, namespace):
    return {
        "apiVersion": "gateway.networking.k8s.io/v1",
        "kind": "Gateway",
        "metadata": {
            "name": name,
            "namespace": namespace,
        },
        "spec": {
            "gatewayClassName": GATEWAY_CLASS_NAME,
            "listeners": [
                {
                    "name": "http",
                    "port": 80,
                    "protocol": "HTTP",
                    "allowedRoutes": {
                        "namespaces": {"from": "All"},
                    },
                },
            ],
        },
    }


def _rewrite_rule(path_prefix, rewrite_to, backend_ref):
    return {
        "matches": [
            {"path": {"type": "PathPrefix", "value": path_prefix}},
        ],
        "filters": [
            {
                "type": "URLRewrite",
                "urlRewrite": {
                    "path": {
                        "replacePrefixMatch": rewrite_to,
                        "type": "ReplacePrefixMatch",
                    },
                },
            },
        ],
        "backendRefs": [backend_ref],
    }


def _pool_ref(service_name, namespace):
    return {
        "group": "inference.networking.k8s.io",
        "kind": "InferencePool",
        "name": f"{service_name}-inference-pool",
        "namespace": namespace,
        "port": 8000,
    }


def _svc_ref(service_name, namespace):
    return {
        "group": "",
        "kind": "Service",
        "name": f"{service_name}-kserve-workload-svc",
        "namespace": namespace,
        "port": 8000,
    }


def make_router_main_route(
    name,
    namespace,
    gateway_name,
    service_name,
):
    pool = _pool_ref(service_name, namespace)
    svc = _svc_ref(service_name, namespace)
    prefix = f"/{namespace}/{service_name}"
    return {
        "apiVersion": "gateway.networking.k8s.io/v1",
        "kind": "HTTPRoute",
        "metadata": {
            "name": name,
            "namespace": namespace,
        },
        "spec": {
            "parentRefs": [
                {
                    "name": gateway_name,
                    "namespace": namespace,
                },
            ],
            "rules": [
                _rewrite_rule(
                    f"{prefix}/v1/completions",
                    "/v1/completions",
                    pool,
                ),
                _rewrite_rule(
                    f"{prefix}/v1/chat/completions",
                    "/v1/chat/completions",
                    pool,
                ),
                _rewrite_rule(prefix, "/", svc),
            ],
        },
    }


def make_router_health_route(
    name,
    namespace,
    gateway_name,
    service_name,
):
    svc = _svc_ref(service_name, namespace)
    prefix = f"/{namespace}/{service_name}"
    return {
        "apiVersion": "gateway.networking.k8s.io/v1",
        "kind": "HTTPRoute",
        "metadata": {
            "name": name,
            "namespace": namespace,
        },
        "spec": {
            "parentRefs": [
                {
                    "name": gateway_name,
                    "namespace": namespace,
                },
            ],
            "rules": [
                _rewrite_rule(
                    f"{prefix}/health",
                    "/health",
                    svc,
                ),
            ],
        },
    }
