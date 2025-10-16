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

from .fixtures import KSERVE_TEST_NAMESPACE

ROUTER_GATEWAYS = [
    {
        "apiVersion": "gateway.networking.k8s.io/v1",
        "kind": "Gateway",
        "metadata": {
            "name": "router-gateway-1",
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "gatewayClassName": "envoy",
            "listeners": [
                {
                    "name": "http",
                    "port": 80,
                    "protocol": "HTTP",
                    "allowedRoutes": {
                        "namespaces": {
                            "from": "All",
                        },
                    },
                },
            ],
        },
    },
    {
        "apiVersion": "gateway.networking.k8s.io/v1",
        "kind": "Gateway",
        "metadata": {
            "name": "router-gateway-2",
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "gatewayClassName": "envoy",
            "listeners": [
                {
                    "name": "http",
                    "port": 80,
                    "protocol": "HTTP",
                    "allowedRoutes": {
                        "namespaces": {
                            "from": "All",
                        },
                    },
                },
            ],
        },
    }
]

ROUTER_ROUTES = [
    {
        "apiVersion": "gateway.networking.k8s.io/v1",
        "kind": "HTTPRoute",
        "metadata": {
            "name": "router-route-1",
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "parentRefs": [
                {
                    "name": "router-gateway-1",
                    "namespace": KSERVE_TEST_NAMESPACE,
                }
            ],
            "rules": [
                {
                    "matches": [
                        {
                            "path": {
                                "type": "PathPrefix",
                                "value": "/kserve-ci-e2e-test/router-with-refs-test",
                            },
                        },
                    ],
                    "filters": [
                        {
                            "type": "URLRewrite",
                            "urlRewrite": {
                                "path": {
                                    "replacePrefixMatch": "/",
                                    "type": "ReplacePrefixMatch",
                                },
                            },
                        },
                    ],
                    "backendRefs": [
                        {
                            "group": "inference.networking.x-k8s.io",
                            "kind": "InferencePool",
                            "name": "router-with-refs-test-inference-pool",
                            "namespace": KSERVE_TEST_NAMESPACE,
                            "port": 8000,
                        }
                    ],
                },
            ],
        },
    },
    {
        "apiVersion": "gateway.networking.k8s.io/v1",
        "kind": "HTTPRoute",
        "metadata": {
            "name": "router-route-2",
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "parentRefs": [
                {
                    "name": "router-gateway-1",
                    "namespace": KSERVE_TEST_NAMESPACE,
                }
            ],
            "rules": [
                {
                    "matches": [
                        {
                            "path": {
                                "type": "PathPrefix",
                                "value": "/kserve-ci-e2e-test/router-with-refs-test/health",
                            },
                        },
                    ],
                    "filters": [
                        {
                            "type": "URLRewrite",
                            "urlRewrite": {
                                "path": {
                                    "replacePrefixMatch": "/health",
                                    "type": "ReplacePrefixMatch",
                                },
                            },
                        },
                    ],
                    "backendRefs": [
                        {
                            "group": "inference.networking.x-k8s.io",
                            "kind": "InferencePool",
                            "name": "router-with-refs-test-inference-pool",
                            "namespace": KSERVE_TEST_NAMESPACE,
                            "port": 8000,
                        }
                    ],
                },
            ],
        },
    },
    {
        "apiVersion": "gateway.networking.k8s.io/v1",
        "kind": "HTTPRoute",
        "metadata": {
            "name": "router-route-3",
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "parentRefs": [
                {
                    "name": "router-gateway-2",
                    "namespace": KSERVE_TEST_NAMESPACE,
                }
            ],
            "rules": [
                {
                    "matches": [
                        {
                            "path": {
                                "type": "PathPrefix",
                                "value": "/kserve-ci-e2e-test/router-with-refs-pd-test",
                            },
                        },
                    ],
                    "filters": [
                        {
                            "type": "URLRewrite",
                            "urlRewrite": {
                                "path": {
                                    "replacePrefixMatch": "/",
                                    "type": "ReplacePrefixMatch",
                                },
                            },
                        },
                    ],
                    "backendRefs": [
                        {
                            "group": "inference.networking.x-k8s.io",
                            "kind": "InferencePool",
                            "name": "router-with-refs-pd-test-inference-pool",
                            "namespace": KSERVE_TEST_NAMESPACE,
                            "port": 8000,
                        }
                    ],
                },
            ],
        },
    },
    {
        "apiVersion": "gateway.networking.k8s.io/v1",
        "kind": "HTTPRoute",
        "metadata": {
            "name": "router-route-4",
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "parentRefs": [
                {
                    "name": "router-gateway-2",
                    "namespace": KSERVE_TEST_NAMESPACE,
                }
            ],
            "rules": [
                {
                    "matches": [
                        {
                            "path": {
                                "type": "PathPrefix",
                                "value": "/kserve-ci-e2e-test/router-with-refs-pd-test/health",
                            },
                        },
                    ],
                    "filters": [
                        {
                            "type": "URLRewrite",
                            "urlRewrite": {
                                "path": {
                                    "replacePrefixMatch": "/health",
                                    "type": "ReplacePrefixMatch",
                                },
                            },
                        },
                    ],
                    "backendRefs": [
                        {
                            "group": "inference.networking.x-k8s.io",
                            "kind": "InferencePool",
                            "name": "router-with-refs-pd-test-inference-pool",
                            "namespace": KSERVE_TEST_NAMESPACE,
                            "port": 8000,
                        }
                    ],
                },
            ],
        },
    }
]
