"""API retries preserve uncertainty without duplicating issue writes."""

import io
from pathlib import Path
import sys
import unittest
from unittest.mock import patch
from urllib.error import HTTPError

sys.path.insert(0, str(Path(__file__).parents[1]))
from linkcheck.api import GitHubAPI


class Response(io.BytesIO):
    status = 200


def failure(code, headers=None):
    return HTTPError(
        "https://api.github.com/repos/o/r", code, "failed", headers or {}, None
    )


@patch("linkcheck.api.time.sleep")
class APITests(unittest.TestCase):
    def test_rate_limits_and_exhausted_outages_do_not_fabricate_later_results(
        self, sleep
    ):
        for code, headers, attempts in (
            (503, {}, 3),
            (429, {}, 1),
            (403, {"X-RateLimit-Remaining": "0"}, 1),
        ):
            with (
                self.subTest(code=code),
                patch(
                    "linkcheck.api.urlopen", side_effect=failure(code, headers)
                ) as request,
            ):
                api = GitHubAPI(token="test")
                self.assertEqual(api.get("/repos/o/r")[0], code)
                self.assertEqual(api.get("/repos/other/repo")[0], -1)
                self.assertEqual(request.call_count, attempts)

    def test_a_404_at_zero_remaining_does_not_fabricate_later_404s(self, sleep):
        with patch(
            "linkcheck.api.urlopen",
            side_effect=[
                failure(404, {"X-RateLimit-Remaining": "0"}),
                Response(b'{"id":1}'),
            ],
        ) as request:
            api = GitHubAPI(token="test")
            self.assertEqual(api.get("/repos/o/r")[0], 404)
            self.assertEqual(api.get("/repos/other/repo")[0], 200)
            self.assertEqual(request.call_count, 2)

    def test_uncertain_issue_creation_is_never_retried_blindly(self, sleep):
        with patch("linkcheck.api.urlopen", side_effect=failure(503)) as request:
            with self.assertRaises(RuntimeError):
                GitHubAPI(token="test").write(
                    "POST", "/repos/o/r/issues", {"title": "example"}
                )
            self.assertEqual(request.call_count, 1)

    def test_reads_retry_outages_and_cache_success(self, sleep):
        with patch(
            "linkcheck.api.urlopen", side_effect=[failure(503), Response(b'{"id":1}')]
        ) as request:
            api = GitHubAPI(token="test")
            self.assertEqual(api.get("/repos/o/r"), (200, {"id": 1}))
            self.assertEqual(api.get("/repos/o/r"), (200, {"id": 1}))
            self.assertEqual(request.call_count, 2)

    def test_missing_targets_are_not_cached_between_confirmations(self, sleep):
        with patch("linkcheck.api.urlopen", side_effect=failure(404)) as request:
            api = GitHubAPI(token="test")
            api.get("/repos/o/r/contents/a.md")
            api.get("/repos/o/r/contents/a.md")
            self.assertEqual(request.call_count, 2)
