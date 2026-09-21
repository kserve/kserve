#!/usr/bin/env python3
"""Run one capture after the runtime is ready and report its result."""

import json
import logging
import os
import signal
import ssl
import subprocess
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone


logging.basicConfig(
    level=logging.INFO,
    format="%(levelname)s: %(message)s",
)
logger = logging.getLogger("mcv-capture")

RESULT_PATH = os.environ.get("MCV_RESULT_PATH", "/tmp/mcv/result.json")
SNAPSHOT_PATH = os.environ.get(
    "MCV_SNAPSHOT_PATH", "/tmp/mcv/cache-snapshot.json"
)
DEFAULT_READINESS_TIMEOUT_SECONDS = 600
SNAPSHOT_RETRY_ATTEMPTS = 3
SNAPSHOT_RETRY_DELAY_SECONDS = 5
COMMAND_POLL_INTERVAL_SECONDS = 0.2
COMMAND_TERMINATION_TIMEOUT_SECONDS = 10
SHUTDOWN_POLL_INTERVAL_SECONDS = 1
SESSION_TEST_FAILURE_MARKERS = (
    "testing value /status/activeSession/id failed: test failed",
    "testing value /status/activeSession/podName failed: test failed",
)

shutdown_event = threading.Event()


class CaptureConfigurationError(RuntimeError):
    """The sidecar configuration cannot be used for capture."""


class ShutdownRequested(Exception):
    """The sidecar received a termination signal."""


def parse_json_env(name):
    value = os.environ.get(name, "")
    if not value:
        return None
    try:
        parsed = json.loads(value)
    except json.JSONDecodeError as error:
        raise CaptureConfigurationError(f"{name} must contain valid JSON") from error
    if not isinstance(parsed, dict):
        raise CaptureConfigurationError(f"{name} must contain a JSON object")
    return parsed


def required_string(config, key, name):
    value = config.get(key)
    if not isinstance(value, str) or not value:
        raise CaptureConfigurationError(f"{name}.{key} is required")
    return value


def validate_capture_config(config):
    if config.get("version") != 1:
        raise CaptureConfigurationError(
            "MCV_CAPTURE_CONFIG has an unsupported version"
        )
    required_string(config, "cacheDir", "MCV_CAPTURE_CONFIG")
    required_string(config, "targetImage", "MCV_CAPTURE_CONFIG")
    capture = config.get("capture")
    if not isinstance(capture, dict):
        raise CaptureConfigurationError(
            "MCV_CAPTURE_CONFIG.capture must be an object"
        )
    for key in ("name", "namespace", "sessionID"):
        required_string(capture, key, "MCV_CAPTURE_CONFIG.capture")
    cache_paths = config.get("cachePaths", [])
    if not isinstance(cache_paths, list):
        raise CaptureConfigurationError(
            "MCV_CAPTURE_CONFIG.cachePaths must be an array"
        )
    return config


def validate_readiness_config(config):
    url = required_string(config, "url", "MCV_READINESS_CONFIG")
    timeout = config.get(
        "mcvCaptureReadinessTimeoutSeconds", DEFAULT_READINESS_TIMEOUT_SECONDS
    )
    if isinstance(timeout, bool):
        raise CaptureConfigurationError(
            "MCV_READINESS_CONFIG.mcvCaptureReadinessTimeoutSeconds must be an integer"
        )
    try:
        timeout = int(timeout)
    except (TypeError, ValueError) as error:
        raise CaptureConfigurationError(
            "MCV_READINESS_CONFIG.mcvCaptureReadinessTimeoutSeconds must be an integer"
        ) from error
    if timeout <= 0:
        raise CaptureConfigurationError(
            "MCV_READINESS_CONFIG.mcvCaptureReadinessTimeoutSeconds must be greater than zero"
        )
    return {"url": url, "mcvCaptureReadinessTimeoutSeconds": timeout}


def validate_runtime_info(config):
    for key, value in config.items():
        if not isinstance(value, str):
            raise CaptureConfigurationError(
                f"MCV_RUNTIME_INFO.{key} must be a string"
            )
    return config


def capture_config():
    config = parse_json_env("MCV_CAPTURE_CONFIG")
    if config is None:
        raise CaptureConfigurationError("MCV_CAPTURE_CONFIG is required")
    return validate_capture_config(config)


def readiness_config():
    config = parse_json_env("MCV_READINESS_CONFIG")
    if config is None:
        raise CaptureConfigurationError("MCV_READINESS_CONFIG is required")
    return validate_readiness_config(config)


def runtime_info():
    config = parse_json_env("MCV_RUNTIME_INFO")
    if config is None:
        raise CaptureConfigurationError("MCV_RUNTIME_INFO is required")
    return validate_runtime_info(config)


def reporter_access_file():
    return os.environ.get("MCV_REPORTER_ACCESS_FILE", "")


def kubernetes_ca_file():
    return os.environ.get(
        "MCV_KUBERNETES_CA_FILE",
        "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
    )


def registry_access_file():
    return os.environ.get("MCV_REGISTRY_ACCESS_FILE", "")


def timestamp():
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def handle_shutdown(_signum, _frame):
    shutdown_event.set()


def install_signal_handlers():
    shutdown_event.clear()
    signal.signal(signal.SIGTERM, handle_shutdown)
    signal.signal(signal.SIGINT, handle_shutdown)


def raise_if_shutdown():
    if shutdown_event.is_set():
        raise ShutdownRequested()


def wait_for_shutdown():
    while not shutdown_event.wait(SHUTDOWN_POLL_INTERVAL_SECONDS):
        pass


def terminate_process(process):
    if process.poll() is not None:
        return
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        process.wait()
        return
    except OSError:
        process.terminate()
    try:
        process.wait(timeout=COMMAND_TERMINATION_TIMEOUT_SECONDS)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            process.wait()
            return
        process.wait()


def run_command(command):
    raise_if_shutdown()
    try:
        process = subprocess.Popen(command, start_new_session=True)
    except OSError:
        if shutdown_event.is_set():
            raise ShutdownRequested()
        raise

    try:
        while True:
            returncode = process.poll()
            if returncode is not None:
                if shutdown_event.is_set():
                    raise ShutdownRequested()
                if returncode != 0:
                    raise subprocess.CalledProcessError(returncode, command)
                return
            if shutdown_event.wait(COMMAND_POLL_INTERVAL_SECONDS):
                raise ShutdownRequested()
    finally:
        if shutdown_event.is_set():
            terminate_process(process)


def wait_for_json_access(path, description):
    if not path:
        raise_if_shutdown()
        raise RuntimeError(f"{description} access file is required")
    logger.info("Waiting for %s access", description)
    while True:
        raise_if_shutdown()
        access = read_json_access(path)
        if access is not None and access.get("token"):
            logger.info("%s access is available", description)
            return access
        if shutdown_event.wait(1):
            raise ShutdownRequested()


def read_json_access(path):
    try:
        with open(path, encoding="utf-8") as file:
            access = json.load(file)
        if isinstance(access, dict):
            return access
    except (FileNotFoundError, json.JSONDecodeError):
        pass
    return None


def is_capture_session_superseded(status_code, response):
    if status_code != 422:
        return False

    try:
        status = json.loads(response)
    except json.JSONDecodeError:
        return False

    if not isinstance(status, dict):
        return False
    if status.get("kind") != "Status":
        return False
    if status.get("status") != "Failure":
        return False
    if status.get("reason") != "Invalid":
        return False
    if status.get("code") != 422:
        return False

    message = status.get("message")
    if not isinstance(message, str):
        return False
    return any(marker in message for marker in SESSION_TEST_FAILURE_MARKERS)


def wait_for_readiness():
    config = readiness_config()
    url = config.get("url", "")
    if not url:
        raise RuntimeError("MCV_READINESS_CONFIG url is required")
    timeout = config["mcvCaptureReadinessTimeoutSeconds"]
    deadline = time.monotonic() + timeout
    logger.info("Waiting for workload readiness probe")
    while True:
        raise_if_shutdown()
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise ReadinessTimeout(
                f"workload readiness probe did not succeed within {timeout} seconds"
            )
        try:
            with urllib.request.urlopen(url, timeout=min(1, remaining)):
                logger.info("Workload readiness probe succeeded")
                return
        except (urllib.error.URLError, TimeoutError):
            if shutdown_event.wait(min(1, max(0, deadline - time.monotonic()))):
                raise ShutdownRequested()


def reporter_request(status):
    config = capture_config()
    capture = config["capture"]
    namespace = capture["namespace"]
    name = capture["name"]
    access = read_json_access(reporter_access_file())
    if access is None or not access.get("token"):
        raise RuntimeError("reporter access is not available")
    if not namespace or not name:
        raise RuntimeError("capture namespace and name are required")

    session_id = capture["sessionID"]
    source_pod_name = os.environ.get("MCV_SOURCE_POD_NAME", "")
    if not session_id or not source_pod_name:
        raise RuntimeError("capture session ID and source Pod name are required")

    path = "/apis/serving.kserve.io/v1alpha1/namespaces/{}/kernelcachecaptures/{}/status".format(
        urllib.parse.quote(namespace, safe=""), urllib.parse.quote(name, safe="")
    )
    request = urllib.request.Request(
        "https://kubernetes.default.svc" + path,
        data=json.dumps([
            {"op": "test", "path": "/status/activeSession/id",
             "value": session_id},
            {"op": "test", "path": "/status/activeSession/podName",
             "value": source_pod_name},
            {"op": "add", "path": "/status/runtimeResult", "value": status},
        ]).encode(),
        headers={
            "Authorization": "Bearer " + access["token"],
            "Content-Type": "application/json-patch+json",
        },
        method="PATCH",
    )
    context = ssl.create_default_context(cafile=kubernetes_ca_file())
    try:
        with urllib.request.urlopen(request, context=context, timeout=10):
            pass
    except urllib.error.HTTPError as error:
        response = error.read().decode("utf-8", errors="replace")
        if error.code in (404, 410):
            raise CaptureTargetGone() from error
        if is_capture_session_superseded(error.code, response):
            raise CaptureSessionSuperseded() from error
        raise RuntimeError(f"HTTP Error {error.code}: {response[:1024]}") from error


class CaptureSessionSuperseded(Exception):
    """The operator has selected another producer."""


class CaptureTargetGone(Exception):
    """The capture target was deleted before the report completed."""


class ReadinessTimeout(RuntimeError):
    """The workload did not become ready before the configured deadline."""


def report(status):
    logger.info("Reporting KernelCacheCapture state: %s", status.get("state", "unknown"))
    attempt = 0
    while True:
        attempt += 1
        raise_if_shutdown()
        try:
            reporter_request(status)
            logger.info("KernelCacheCapture state reported: %s", status.get("state", "unknown"))
            return
        except CaptureConfigurationError as error:
            logger.error("KernelCacheCapture configuration is invalid: %s", error)
            return
        except (OSError, urllib.error.URLError, urllib.error.HTTPError, RuntimeError) as error:
            if attempt == 1 or attempt % 12 == 0:
                logger.warning("KernelCacheCapture state report failed; retrying: %s", error)
            if shutdown_event.wait(5):
                raise ShutdownRequested()


def capture_result():
    config = capture_config()
    with open(RESULT_PATH, encoding="utf-8") as file:
        result = json.load(file)
    if not isinstance(result, dict):
        raise RuntimeError("MCV create result must be a JSON object")
    result = {
        key: json.dumps(value, separators=(",", ":"))
        if isinstance(value, (dict, list))
        else str(value)
        for key, value in result.items()
        if value is not None
    }
    result["cachePaths"] = json.dumps(
        config.get("cachePaths", []), separators=(",", ":")
    )
    runtime_metadata = runtime_info()
    if runtime_metadata:
        result["runtimeInfo"] = json.dumps(runtime_metadata, separators=(",", ":"), sort_keys=True)
    result["sourcePodName"] = os.environ.get("MCV_SOURCE_POD_NAME", "")
    result["captureSessionID"] = config["capture"]["sessionID"]
    result.setdefault("capturedAt", timestamp())
    return result


def failure_result(reason, message):
    try:
        config = capture_config()
    except RuntimeError:
        config = {"capture": {}}
    return {
        "state": "Failed",
        "reason": reason,
        "message": message[:1024],
        "completedAt": timestamp(),
        "sourcePodName": os.environ.get("MCV_SOURCE_POD_NAME", ""),
        "captureSessionID": config["capture"].get("sessionID", ""),
    }


def wait_for_registry_access():
    access_file = registry_access_file()
    if access_file:
        logger.info("Waiting for registry access")
        wait_for_json_access(access_file, "registry")


def create_snapshot(cache_dir):
    logger.info("Creating baseline cache directory snapshot")
    command = [
        "/mcv", "--snapshot", "--dir", cache_dir,
        "--snapshot-file", SNAPSHOT_PATH,
    ]
    for attempt in range(1, SNAPSHOT_RETRY_ATTEMPTS + 1):
        try:
            run_command(command)
            logger.info("Baseline cache directory snapshot created")
            return
        except subprocess.CalledProcessError as error:
            if attempt == SNAPSHOT_RETRY_ATTEMPTS:
                raise RuntimeError(
                    f"MCV cache snapshot failed after {attempt} attempts "
                    f"with exit code {error.returncode}"
                ) from error
            logger.warning(
                "MCV cache snapshot failed with exit code %s; retrying (%s/%s)",
                error.returncode,
                attempt,
                SNAPSHOT_RETRY_ATTEMPTS,
            )
            if shutdown_event.wait(SNAPSHOT_RETRY_DELAY_SECONDS):
                raise ShutdownRequested()


def main():
    logger.info("KernelCache capture sidecar started")
    install_signal_handlers()
    try:
        config = capture_config()
        cache_dir = config.get("cacheDir", "")
        if not cache_dir:
            raise RuntimeError("MCV_CAPTURE_CONFIG cacheDir is required for capture")
        create_snapshot(cache_dir)
        wait_for_json_access(reporter_access_file(), "reporter")
        capture_session_id = config["capture"]["sessionID"]
        report({
            "state": "WaitingForWorkload",
            "capturedAt": timestamp(),
            "sourcePodName": os.environ.get("MCV_SOURCE_POD_NAME", ""),
            "captureSessionID": capture_session_id,
        })
        wait_for_readiness()
        report({
            "state": "Capturing",
            "capturedAt": timestamp(),
            "sourcePodName": os.environ.get("MCV_SOURCE_POD_NAME", ""),
            "captureSessionID": capture_session_id,
        })
        wait_for_registry_access()

        target_image = config.get("targetImage", "")
        if not target_image:
            raise RuntimeError("MCV_CAPTURE_CONFIG targetImage is required for capture")
        logger.info("Starting OCI cache image creation")
        run_command([
            "/mcv", "--create", "--builder", "oci", "--image", target_image,
            "--dir", cache_dir, "--result", RESULT_PATH,
            "--delta-from-snapshot", "--snapshot-file", SNAPSHOT_PATH,
        ])
        result = capture_result()
        if result.get("state") == "Unchanged":
            logger.info("No new cache directories found; OCI image creation skipped")
        else:
            logger.info("OCI cache image creation completed")
        report(result)
    except ShutdownRequested:
        logger.info("Shutdown requested; stopping capture sidecar")
        return
    except subprocess.CalledProcessError as error:
        logger.error("OCI cache image creation failed with exit code %s", error.returncode)
        report(failure_result("CaptureFailed", "MCV OCI image creation failed"))
    except ReadinessTimeout as error:
        logger.error("KernelCache readiness wait failed: %s", error)
        report(failure_result("CaptureFailed", str(error)))
    except (OSError, RuntimeError, json.JSONDecodeError) as error:
        logger.error("KernelCache capture failed: %s", error)
        report(failure_result("CaptureFailed", "KernelCache capture failed"))

    logger.info("KernelCache capture sidecar is idle")
    wait_for_shutdown()


if __name__ == "__main__":
    try:
        main()
    except CaptureSessionSuperseded:
        logger.info("Capture session superseded; sidecar is idle")
        wait_for_shutdown()
    except CaptureTargetGone:
        logger.info("KernelCacheCapture no longer exists; sidecar is idle")
        wait_for_shutdown()
    except ShutdownRequested:
        logger.info("Shutdown requested; stopping capture sidecar")
