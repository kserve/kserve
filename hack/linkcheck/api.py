#!/usr/bin/env python3

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

"""Small GitHub API client with bounded retries and per-run response caching."""

import json
import os
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


class GitHubAPI:
    def __init__(self, token=None):
        self.token = token if token is not None else os.environ.get("GITHUB_TOKEN")
        self.cache = {}
        self.last_request = 0
        self.rate_limit = None
        self.unavailable = None

    def request(self, method, path, payload=None):
        if not path.startswith("/") or path.startswith("//"):
            raise ValueError("Expected a GitHub API path")
        if method == "GET" and path in self.cache:
            return self.cache[path]
        if self.rate_limit is not None:
            return -1, {"message": "GitHub API rate limit reached in this audit"}
        if self.unavailable is not None:
            return -1, {"message": "GitHub API unavailable in this audit"}
        headers = {
            "Accept": "application/vnd.github+json",
            "User-Agent": "KServe-link-check",
        }
        if self.token:
            headers["Authorization"] = "Bearer " + self.token
        data = json.dumps(payload).encode() if payload is not None else None
        if data is not None:
            headers["Content-Type"] = "application/json"
        for attempt in range(3):
            time.sleep(max(0, self.last_request + 0.2 - time.monotonic()))
            self.last_request = time.monotonic()
            request = Request(
                "https://api.github.com" + path,
                data=data,
                headers=headers,
                method=method,
            )
            try:
                with urlopen(request, timeout=20) as response:
                    status, result = response.status, json.load(response)
            except HTTPError as error:
                status, result = error.code, {"message": f"GitHub HTTP {error.code}"}
                if (
                    status == 429
                    or status == 403
                    and (error.headers or {}).get("X-RateLimit-Remaining") == "0"
                ):
                    self.rate_limit = status
            except (URLError, TimeoutError, OSError):
                status, result = 0, {"message": "GitHub API connection failed"}
            if method == "GET" and (status == 0 or status >= 500):
                if attempt < 2:
                    time.sleep(2**attempt)
                    continue
            break
        # Missing targets and empty ref lists must be requested again when
        # confirming a failure. Cache successful, nonempty GET results only.
        if method == "GET" and status == 200 and result:
            self.cache[path] = status, result
        elif method == "GET" and (status == 0 or status >= 500):
            # One exhausted API request is enough to pause remaining GitHub
            # verification. An API outage must not cause hundreds of sequential
            # timeouts and a failed audit instead of inconclusive observations.
            self.unavailable = status
        return status, result

    def get(self, path):
        return self.request("GET", path)

    def write(self, method, path, payload):
        status, result = self.request(method, path, payload)
        if status not in (200, 201):
            raise RuntimeError(f"GitHub issue update failed: HTTP {status}")
        return result
