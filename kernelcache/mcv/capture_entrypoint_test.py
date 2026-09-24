import importlib.util
import json
import os
import pathlib
import signal
import tempfile
import unittest
from unittest import mock


ENTRYPOINT = pathlib.Path(__file__).with_name("capture-entrypoint.py")
SPEC = importlib.util.spec_from_file_location("capture_entrypoint", ENTRYPOINT)
capture_entrypoint = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(capture_entrypoint)


GROUPED_CAPTURE_CONFIG = {
    "version": 1,
    "cacheDir": "/workspace/cache/0",
    "targetImage": "registry.example/cache:session",
    "capture": {
        "name": "capture",
        "namespace": "team",
        "sessionID": "session-id",
    },
    "cachePaths": [
        {
            "containerName": "kserve-container",
            "containerPath": "/tmp/vllm",
            "ociPath": "io.vllm.cache",
        }
    ],
}
READINESS_CONFIG = {
    "url": "http://127.0.0.1:8080/health",
    "mcvCaptureReadinessTimeoutSeconds": 600,
}
RUNTIME_INFO = {
    "commandHash": "command-hash",
    "argsHash": "args-hash",
    "modelURIHash": "model-hash",
}


def grouped_environment():
    return {
        "MCV_CAPTURE_CONFIG": json.dumps(GROUPED_CAPTURE_CONFIG),
        "MCV_READINESS_CONFIG": json.dumps(READINESS_CONFIG),
        "MCV_RUNTIME_INFO": json.dumps(RUNTIME_INFO),
        "MCV_SOURCE_POD_NAME": "model-pod",
    }


class FakeHTTPResponse:
    def __init__(self, payload):
        self.payload = payload

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        return False

    def read(self):
        return json.dumps(self.payload).encode("utf-8")


class CaptureEntrypointTest(unittest.TestCase):
    def setUp(self):
        capture_entrypoint.shutdown_event.clear()

    def tearDown(self):
        capture_entrypoint.shutdown_event.clear()

    def test_grouped_configuration_is_read(self):
        with mock.patch.dict(os.environ, grouped_environment(), clear=True):
            self.assertEqual(
                GROUPED_CAPTURE_CONFIG,
                capture_entrypoint.capture_config(),
            )
            self.assertEqual(
                READINESS_CONFIG,
                capture_entrypoint.readiness_config(),
            )
            self.assertEqual(RUNTIME_INFO, capture_entrypoint.runtime_info())

    def test_grouped_configuration_is_required(self):
        for name, reader in (
            ("MCV_CAPTURE_CONFIG", capture_entrypoint.capture_config),
            ("MCV_READINESS_CONFIG", capture_entrypoint.readiness_config),
            ("MCV_RUNTIME_INFO", capture_entrypoint.runtime_info),
        ):
            with self.subTest(name=name), mock.patch.dict(os.environ, {}, clear=True):
                with self.assertRaisesRegex(
                    capture_entrypoint.CaptureConfigurationError, f"{name} is required"
                ):
                    reader()

    def test_invalid_capture_configuration_is_rejected(self):
        invalid_values = (
            "not-json",
            json.dumps({"version": 2}),
            json.dumps({**GROUPED_CAPTURE_CONFIG, "capture": "invalid"}),
        )
        for value in invalid_values:
            with (
                self.subTest(value=value),
                mock.patch.dict(
                    os.environ,
                    {"MCV_CAPTURE_CONFIG": value},
                    clear=True,
                ),
            ):
                with self.assertRaises(capture_entrypoint.CaptureConfigurationError):
                    capture_entrypoint.capture_config()

    def test_invalid_readiness_configuration_is_rejected(self):
        invalid_values = (
            {"url": 123},
            {**READINESS_CONFIG, "mcvCaptureReadinessTimeoutSeconds": 0},
        )
        for invalid in invalid_values:
            with (
                self.subTest(invalid=invalid),
                mock.patch.dict(
                    os.environ,
                    {"MCV_READINESS_CONFIG": json.dumps(invalid)},
                    clear=True,
                ),
            ):
                with self.assertRaises(capture_entrypoint.CaptureConfigurationError):
                    capture_entrypoint.readiness_config()

    def test_readiness_wait_stops_at_configured_timeout(self):
        with (
            mock.patch.dict(
                os.environ,
                {"MCV_READINESS_CONFIG": json.dumps(READINESS_CONFIG)},
                clear=True,
            ),
            mock.patch.object(
                capture_entrypoint.time, "monotonic", side_effect=[0, 601]
            ),
            mock.patch.object(capture_entrypoint.urllib.request, "urlopen") as urlopen,
        ):
            with self.assertRaises(capture_entrypoint.ReadinessTimeout):
                capture_entrypoint.wait_for_readiness()

        urlopen.assert_not_called()

    def test_snapshot_retries_after_transient_failure(self):
        failed_process = mock.Mock()
        failed_process.poll.return_value = 4
        successful_process = mock.Mock()
        successful_process.poll.return_value = 0
        with (
            mock.patch.object(
                capture_entrypoint.subprocess,
                "Popen",
                side_effect=[failed_process, successful_process],
            ) as run,
            mock.patch.object(
                capture_entrypoint.shutdown_event, "wait", return_value=False
            ) as wait,
        ):
            capture_entrypoint.create_snapshot("/workspace/cache/0")

        self.assertEqual(2, run.call_count)
        wait.assert_called_once_with(capture_entrypoint.SNAPSHOT_RETRY_DELAY_SECONDS)

    def test_snapshot_fails_after_retry_limit(self):
        failure = mock.Mock()
        failure.poll.return_value = 4
        with (
            mock.patch.object(
                capture_entrypoint.subprocess,
                "Popen",
                return_value=failure,
            ) as run,
            mock.patch.object(
                capture_entrypoint.shutdown_event, "wait", return_value=False
            ) as wait,
        ):
            with self.assertRaisesRegex(
                RuntimeError,
                f"after {capture_entrypoint.SNAPSHOT_RETRY_ATTEMPTS} attempts",
            ):
                capture_entrypoint.create_snapshot("/workspace/cache/0")

        self.assertEqual(capture_entrypoint.SNAPSHOT_RETRY_ATTEMPTS, run.call_count)
        self.assertEqual(
            capture_entrypoint.SNAPSHOT_RETRY_ATTEMPTS - 1,
            wait.call_count,
        )

    def test_idle_wait_stops_when_shutdown_is_requested(self):
        with mock.patch.object(
            capture_entrypoint.shutdown_event, "wait", return_value=True
        ) as wait:
            capture_entrypoint.wait_for_shutdown()

        wait.assert_called_once_with(capture_entrypoint.SHUTDOWN_POLL_INTERVAL_SECONDS)

    def test_shutdown_signal_sets_event(self):
        capture_entrypoint.handle_shutdown(signal.SIGTERM, None)

        self.assertTrue(capture_entrypoint.shutdown_event.is_set())

    def test_command_is_terminated_as_a_process_group(self):
        process = mock.Mock(pid=1234)
        process.poll.side_effect = [None, None]

        def request_shutdown(_timeout):
            capture_entrypoint.shutdown_event.set()
            return True

        with (
            mock.patch.object(
                capture_entrypoint.subprocess, "Popen", return_value=process
            ) as popen,
            mock.patch.object(
                capture_entrypoint.shutdown_event, "wait", side_effect=request_shutdown
            ),
            mock.patch.object(capture_entrypoint.os, "killpg") as killpg,
        ):
            with self.assertRaises(capture_entrypoint.ShutdownRequested):
                capture_entrypoint.run_command(["/mcv", "--create"])

        popen.assert_called_once_with(["/mcv", "--create"], start_new_session=True)
        killpg.assert_called_once_with(1234, signal.SIGTERM)
        process.wait.assert_called_once_with(
            timeout=capture_entrypoint.COMMAND_TERMINATION_TIMEOUT_SECONDS
        )

    def test_capture_result_uses_grouped_metadata(self):
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8") as file:
            json.dump({"state": "Succeeded"}, file)
            file.flush()
            with (
                mock.patch.dict(os.environ, grouped_environment(), clear=True),
                mock.patch.object(capture_entrypoint, "RESULT_PATH", file.name),
            ):
                result = capture_entrypoint.capture_result()

        self.assertEqual(
            json.dumps(GROUPED_CAPTURE_CONFIG["cachePaths"], separators=(",", ":")),
            result["cachePaths"],
        )
        self.assertEqual(
            json.dumps(RUNTIME_INFO, separators=(",", ":"), sort_keys=True),
            result["runtimeInfo"],
        )
        self.assertEqual("model-pod", result["sourcePodName"])
        self.assertEqual("session-id", result["captureSessionID"])

    def test_reporter_request_allows_same_pod_and_session_after_restart(self):
        requests = []
        current_result = {
            "state": "Capturing",
            "sourcePodName": "model-pod",
            "captureSessionID": "session-id",
        }

        def urlopen(request, **_kwargs):
            requests.append(request)
            if request.get_method() == "GET":
                return FakeHTTPResponse(
                    {
                        "status": {
                            "activeSession": {
                                "id": "session-id",
                                "podName": "model-pod",
                            },
                            "runtimeResult": current_result,
                        }
                    }
                )
            return FakeHTTPResponse({})

        with (
            mock.patch.dict(os.environ, grouped_environment(), clear=True),
            mock.patch.object(
                capture_entrypoint, "read_json_access", return_value={"token": "token"}
            ),
            mock.patch.object(
                capture_entrypoint.ssl,
                "create_default_context",
                return_value=mock.Mock(),
            ),
            mock.patch.object(
                capture_entrypoint.urllib.request, "urlopen", side_effect=urlopen
            ),
        ):
            capture_entrypoint.reporter_request({"state": "Capturing"})

        self.assertEqual(2, len(requests))
        self.assertEqual("PUT", requests[1].get_method())
        body = json.loads(requests[1].data.decode("utf-8"))
        self.assertEqual("model-pod", body["status"]["runtimeResult"]["sourcePodName"])
        self.assertEqual(
            "session-id", body["status"]["runtimeResult"]["captureSessionID"]
        )

    def test_reporter_request_rejects_different_pod_or_session(self):
        for source_pod_name, session_id in (
            ("other-pod", "session-id"),
            ("model-pod", "other-session"),
        ):
            with self.subTest(source_pod_name=source_pod_name, session_id=session_id):
                current_result = {
                    "state": "Capturing",
                    "sourcePodName": source_pod_name,
                    "captureSessionID": session_id,
                }
                requests = []

                def urlopen(
                    request,
                    _requests=requests,
                    _current_result=current_result,
                    **_kwargs,
                ):
                    _requests.append(request)
                    return FakeHTTPResponse(
                        {
                            "status": {
                                "activeSession": {
                                    "id": "session-id",
                                    "podName": "model-pod",
                                },
                                "runtimeResult": _current_result,
                            }
                        }
                    )

                with (
                    mock.patch.dict(os.environ, grouped_environment(), clear=True),
                    mock.patch.object(
                        capture_entrypoint,
                        "read_json_access",
                        return_value={"token": "token"},
                    ),
                    mock.patch.object(
                        capture_entrypoint.ssl,
                        "create_default_context",
                        return_value=mock.Mock(),
                    ),
                    mock.patch.object(
                        capture_entrypoint.urllib.request,
                        "urlopen",
                        side_effect=urlopen,
                    ),
                ):
                    with self.assertRaises(capture_entrypoint.CaptureSessionSuperseded):
                        capture_entrypoint.reporter_request({"state": "Capturing"})

                self.assertEqual(1, len(requests))

    def test_reporter_request_rejects_different_active_session_before_first_result(
        self,
    ):
        for pod_name, session_id in (
            ("other-pod", "session-id"),
            ("model-pod", "other-session"),
        ):
            with self.subTest(pod_name=pod_name, session_id=session_id):
                requests = []

                def urlopen(
                    request,
                    _requests=requests,
                    _session_id=session_id,
                    _pod_name=pod_name,
                    **_kwargs,
                ):
                    _requests.append(request)
                    return FakeHTTPResponse(
                        {
                            "status": {
                                "activeSession": {
                                    "id": _session_id,
                                    "podName": _pod_name,
                                }
                            }
                        }
                    )

                with (
                    mock.patch.dict(os.environ, grouped_environment(), clear=True),
                    mock.patch.object(
                        capture_entrypoint,
                        "read_json_access",
                        return_value={"token": "token"},
                    ),
                    mock.patch.object(
                        capture_entrypoint.ssl,
                        "create_default_context",
                        return_value=mock.Mock(),
                    ),
                    mock.patch.object(
                        capture_entrypoint.urllib.request,
                        "urlopen",
                        side_effect=urlopen,
                    ),
                ):
                    with self.assertRaises(capture_entrypoint.CaptureSessionSuperseded):
                        capture_entrypoint.reporter_request({"state": "Capturing"})

                self.assertEqual(1, len(requests))

    def test_reporter_request_allows_first_result_without_active_session(self):
        requests = []

        def urlopen(request, **_kwargs):
            requests.append(request)
            if request.get_method() == "GET":
                return FakeHTTPResponse({"status": {}})
            return FakeHTTPResponse({})

        with (
            mock.patch.dict(os.environ, grouped_environment(), clear=True),
            mock.patch.object(
                capture_entrypoint, "read_json_access", return_value={"token": "token"}
            ),
            mock.patch.object(
                capture_entrypoint.ssl,
                "create_default_context",
                return_value=mock.Mock(),
            ),
            mock.patch.object(
                capture_entrypoint.urllib.request, "urlopen", side_effect=urlopen
            ),
        ):
            capture_entrypoint.reporter_request({"state": "Capturing"})

        self.assertEqual(2, len(requests))
        self.assertEqual("PUT", requests[1].get_method())

    def test_reporter_request_allows_first_result_with_active_session(self):
        requests = []

        def urlopen(request, **_kwargs):
            requests.append(request)
            if request.get_method() == "GET":
                return FakeHTTPResponse(
                    {
                        "status": {
                            "activeSession": {
                                "id": "session-id",
                                "podName": "model-pod",
                            }
                        }
                    }
                )
            return FakeHTTPResponse({})

        with (
            mock.patch.dict(os.environ, grouped_environment(), clear=True),
            mock.patch.object(
                capture_entrypoint, "read_json_access", return_value={"token": "token"}
            ),
            mock.patch.object(
                capture_entrypoint.ssl,
                "create_default_context",
                return_value=mock.Mock(),
            ),
            mock.patch.object(
                capture_entrypoint.urllib.request, "urlopen", side_effect=urlopen
            ),
        ):
            capture_entrypoint.reporter_request({"state": "Capturing"})

        self.assertEqual(2, len(requests))
        self.assertEqual("PUT", requests[1].get_method())

    def test_report_does_not_retry_configuration_errors(self):
        with mock.patch.object(
            capture_entrypoint,
            "reporter_request",
            side_effect=capture_entrypoint.CaptureConfigurationError("invalid"),
        ) as reporter_request:
            capture_entrypoint.report({"state": "Failed"})

        reporter_request.assert_called_once_with({"state": "Failed"})

    def test_capture_session_superseded_detection_requires_claim_marker(self):
        cases = (
            (
                400,
                {
                    "kind": "Status",
                    "status": "Failure",
                    "reason": "BadRequest",
                    "code": 400,
                    "message": "runtimeResult.sourcePodName is already claimed",
                },
                True,
            ),
            (
                403,
                {
                    "kind": "Status",
                    "status": "Failure",
                    "reason": "Forbidden",
                    "code": 403,
                    "message": "reporter identity is not authorized for this KernelCacheCapture",
                },
                True,
            ),
            (
                409,
                {
                    "kind": "Status",
                    "status": "Failure",
                    "reason": "Conflict",
                    "code": 409,
                    "message": "runtimeResult.sourcePodName is already claimed",
                },
                True,
            ),
            (
                422,
                {
                    "kind": "Status",
                    "status": "Failure",
                    "reason": "Invalid",
                    "code": 422,
                    "message": "reporter identity is not authorized for this KernelCacheCapture",
                },
                True,
            ),
            (
                422,
                {
                    "kind": "Status",
                    "status": "Failure",
                    "reason": "Invalid",
                    "code": 422,
                    "message": "admission webhook rejected request: test failed",
                },
                False,
            ),
            (422, "not-json", False),
        )
        for status_code, response, expected in cases:
            with self.subTest(status_code=status_code, response=response):
                body = response if isinstance(response, str) else json.dumps(response)
                self.assertEqual(
                    expected,
                    capture_entrypoint.is_capture_session_superseded(status_code, body),
                )

    def test_report_stops_when_capture_session_is_superseded(self):
        with mock.patch.object(
            capture_entrypoint,
            "reporter_request",
            side_effect=capture_entrypoint.CaptureSessionSuperseded(),
        ):
            with self.assertRaises(capture_entrypoint.CaptureSessionSuperseded):
                capture_entrypoint.report({"state": "Capturing"})

    def test_report_stops_when_capture_target_is_deleted(self):
        with mock.patch.object(
            capture_entrypoint,
            "reporter_request",
            side_effect=capture_entrypoint.CaptureTargetGone(),
        ):
            with self.assertRaises(capture_entrypoint.CaptureTargetGone):
                capture_entrypoint.report({"state": "Capturing"})


if __name__ == "__main__":
    unittest.main()
