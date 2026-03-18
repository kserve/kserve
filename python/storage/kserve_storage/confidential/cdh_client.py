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

import logging
import os
import re

import requests

from .secret_resolver import SecretResolutionError, SecretResolver

logger = logging.getLogger(__name__)

# kbs:///<repo>/<type>/<tag>
_KBS_RESOURCE_ID_RE = re.compile(r"^kbs:///(?P<repo>[^/]+)/(?P<type>[^/]+)/(?P<tag>[^/]+)$")

_DEFAULT_CDH_ADDR = "http://127.0.0.1:8006"


class CDHSecretResolver(SecretResolver):
    """Resolves decryption keys via the Confidential Data Hub (CDH).

    CDH is a component of the Confidential Containers (CoCo) guest that runs
    inside the TEE and provides a local API for retrieving secrets.  CDH handles
    attestation transparently — it communicates with the configured Key Broker
    Service (KBS) backend, regardless of the specific RATS protocol or KBS
    implementation (e.g., Trustee, Intel Trust Authority).

    The CDH address is read from the ``CDH_ADDR`` environment variable, defaulting
    to ``http://127.0.0.1:8006``.
    """

    def __init__(self, cdh_addr: str | None = None, timeout: int = 30):
        self._cdh_addr = (cdh_addr or os.environ.get("CDH_ADDR", _DEFAULT_CDH_ADDR)).rstrip("/")
        self._timeout = timeout

    def resolve_key(self, resource_id: str) -> bytes:
        """Retrieve a decryption key from CDH for the given resource identifier.

        Args:
            resource_id: A KBS resource URI in the format ``kbs:///<repo>/<type>/<tag>``.

        Returns:
            The raw key bytes.

        Raises:
            SecretResolutionError: If the resource ID is malformed, CDH is
                unreachable, or the key cannot be retrieved.
        """
        match = _KBS_RESOURCE_ID_RE.match(resource_id)
        if not match:
            raise SecretResolutionError(
                f"Invalid resource ID format: {resource_id!r}, "
                "expected kbs:///<repo>/<type>/<tag>"
            )

        repo = match.group("repo")
        rtype = match.group("type")
        tag = match.group("tag")

        url = f"{self._cdh_addr}/cdh/resource/{repo}/{rtype}/{tag}"
        logger.info("Requesting key from CDH: %s", url)

        try:
            response = requests.get(url, timeout=self._timeout)
            response.raise_for_status()
        except requests.RequestException as e:
            raise SecretResolutionError(f"Failed to retrieve key from CDH: {e}") from e

        return response.content
