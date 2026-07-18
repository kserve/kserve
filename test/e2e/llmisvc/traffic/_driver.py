"""Background HTTP traffic generator with per-request recording."""

from __future__ import annotations

import json
import threading
import time
from typing import Any, Dict, Optional, Union

import requests

from ._report import ResponseRecord, TrafficReport


class TrafficDriver:
    """Sends HTTP requests at a controlled rate in a background thread.

    Sends one request at a time at a target rate. If a request takes
    longer than the inter-request interval, the next send is immediate
    (no queuing, achieved rate drops).

    Payload: if `payload` is a dict, it is serialized with json.dumps()
    and Content-Type is set to application/json (unless overridden in headers).

    Headers: response headers are stored with lowercased keys.
    """

    def __init__(
        self,
        url: str,
        *,
        method: str = "POST",
        headers: Optional[Dict[str, str]] = None,
        payload: Optional[Union[str, bytes, Dict[str, Any]]] = None,
        rate: float = 2.0,
        timeout: float = 5.0,
        session: Optional[requests.Session] = None,
    ) -> None:
        self._url = url
        self._method = method.upper()
        self._timeout = timeout
        self._rate = rate
        self._owns_session = session is None
        self._session = session or requests.Session()

        self._headers: Dict[str, str] = dict(headers or {})
        if isinstance(payload, dict):
            self._payload: Optional[Union[str, bytes]] = json.dumps(payload)
            self._headers.setdefault("Content-Type", "application/json")
        else:
            self._payload = payload

        self._records: list[ResponseRecord] = []
        self._marks: Dict[str, float] = {}
        self._lock = threading.Lock()
        self._stop_event = threading.Event()
        self._thread: Optional[threading.Thread] = None
        self._started = False
        self._stopped = False
        self._report: Optional[TrafficReport] = None
        self._request_count = 0

    def start(
        self, *, warmup: bool = False, warmup_timeout: float = 60.0
    ) -> TrafficDriver:
        """Start sending traffic. Returns self for chaining."""
        if self._started:
            raise RuntimeError("Driver already started")

        if warmup:
            self._warmup(warmup_timeout)

        self._started = True
        self._thread = threading.Thread(target=self._run, daemon=True)
        self._thread.start()
        return self

    def stop(self) -> TrafficReport:
        """Stop traffic and return the report. Idempotent."""
        if self._stopped and self._report is not None:
            return self._report

        if not self._started:
            self._report = TrafficReport([], {}, time.monotonic())
            self._stopped = True
            return self._report

        self._stop_event.set()
        self._thread.join(timeout=self._timeout + 1)

        stop_ts = time.monotonic()
        with self._lock:
            self._report = TrafficReport(
                list(self._records), dict(self._marks), stop_ts
            )

        if self._owns_session:
            self._session.close()

        self._stopped = True
        return self._report

    def mark(self, name: str) -> None:
        """Record a named timestamp for phase-based analysis."""
        ts = time.monotonic()
        with self._lock:
            self._marks[name] = ts

    @property
    def is_running(self) -> bool:
        return self._started and not self._stopped

    @property
    def request_count(self) -> int:
        return self._request_count

    @property
    def report(self) -> TrafficReport:
        """Access after stop(). Raises RuntimeError if still running."""
        if self.is_running:
            raise RuntimeError(
                "Cannot access report while driver is running. Call stop() first."
            )
        if self._report is None:
            raise RuntimeError("Driver was never started.")
        return self._report

    def _warmup(self, timeout: float) -> None:
        """Send probe requests until first 2xx. Not included in report."""
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            try:
                resp = self._session.request(
                    self._method,
                    self._url,
                    headers=self._headers,
                    data=self._payload,
                    timeout=self._timeout,
                )
                if 200 <= resp.status_code < 300:
                    return
            except requests.RequestException:
                pass
            time.sleep(1)
        raise TimeoutError(
            f"Warmup failed: no successful response within {timeout}s from {self._url}"
        )

    def _run(self) -> None:
        """Send loop - runs in background thread."""
        interval = 1.0 / self._rate
        next_tick = time.monotonic()

        while not self._stop_event.is_set():
            now = time.monotonic()
            if now < next_tick:
                wait_time = next_tick - now
                if self._stop_event.wait(wait_time):
                    break

            next_tick += interval
            record = self._send_one()
            self._request_count += 1
            with self._lock:
                self._records.append(record)

    def _send_one(self) -> ResponseRecord:
        """Send a single request and record the result."""
        ts = time.monotonic()
        wall = time.time()
        try:
            resp = self._session.request(
                self._method,
                self._url,
                headers=self._headers,
                data=self._payload,
                timeout=self._timeout,
            )
            latency = time.monotonic() - ts
            resp_headers = {k.lower(): v for k, v in resp.headers.items()}
            body = None
            if not (200 <= resp.status_code < 300):
                body = resp.text[:512] if resp.text else None
            return ResponseRecord(
                timestamp=ts,
                wall_clock=wall,
                status=resp.status_code,
                latency=latency,
                headers=resp_headers,
                body=body,
            )
        except requests.RequestException as e:
            latency = time.monotonic() - ts
            return ResponseRecord(
                timestamp=ts,
                wall_clock=wall,
                status=0,
                latency=latency,
                error=str(e),
            )
