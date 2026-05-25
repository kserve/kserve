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

from kserve.model import append_forwardable_headers, _FORWARDABLE_HEADERS


class TestFilterHeaders:
    def test_none_headers_returns_base(self):
        result = append_forwardable_headers(None, {"Content-Type": "application/json"})
        assert result == {"Content-Type": "application/json"}

    def test_none_headers_no_base_returns_empty(self):
        result = append_forwardable_headers(None)
        assert result == {}

    def test_empty_headers_returns_base(self):
        result = append_forwardable_headers({}, {"Content-Type": "application/json"})
        assert result == {"Content-Type": "application/json"}

    def test_forwards_authorization(self):
        headers = {"authorization": "Bearer token123", "host": "transformer-host"}
        result = append_forwardable_headers(
            headers, {"Content-Type": "application/json"}
        )
        assert result == {
            "Content-Type": "application/json",
            "authorization": "Bearer token123",
        }
        assert "host" not in result

    def test_forwards_x_request_id(self):
        headers = {"x-request-id": "abc-123"}
        result = append_forwardable_headers(headers)
        assert result == {"x-request-id": "abc-123"}

    def test_forwards_x_b3_traceid(self):
        headers = {"x-b3-traceid": "trace-456"}
        result = append_forwardable_headers(headers)
        assert result == {"x-b3-traceid": "trace-456"}

    def test_forwards_all_allowed_headers(self):
        headers = {
            "x-request-id": "req-1",
            "x-b3-traceid": "trace-2",
            "authorization": "Bearer xyz",
            "host": "should-be-dropped",
            "x-custom": "should-be-dropped",
        }
        result = append_forwardable_headers(
            headers, {"Content-Type": "application/json"}
        )
        assert result == {
            "Content-Type": "application/json",
            "x-request-id": "req-1",
            "x-b3-traceid": "trace-2",
            "authorization": "Bearer xyz",
        }

    def test_drops_non_forwardable_headers(self):
        headers = {
            "host": "transformer-host",
            "content-length": "42",
            "x-custom-header": "value",
            "cookie": "session=abc",
        }
        result = append_forwardable_headers(headers)
        assert result == {}

    def test_base_not_mutated(self):
        base = {"Content-Type": "application/json"}
        append_forwardable_headers({"authorization": "Bearer tok"}, base)
        assert base == {"Content-Type": "application/json"}

    def test_forwardable_headers_contains_expected_keys(self):
        assert "x-request-id" in _FORWARDABLE_HEADERS
        assert "x-b3-traceid" in _FORWARDABLE_HEADERS
        assert "authorization" in _FORWARDABLE_HEADERS
