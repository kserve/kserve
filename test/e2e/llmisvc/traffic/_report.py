"""Data model and queryable views for traffic driver results."""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass, field
from typing import Callable, Dict, List, Optional, Sequence


@dataclass(frozen=True)
class ResponseRecord:
    """Single HTTP response captured by the driver."""

    timestamp: float  # time.monotonic()
    wall_clock: float  # time.time()
    status: int  # HTTP status code (0 for connection errors)
    latency: float  # seconds
    headers: Dict[str, str] = field(default_factory=dict)  # lowercased keys
    error: Optional[str] = None
    body: Optional[str] = None  # response body for non-2xx (truncated to 512 chars)

    @property
    def is_success(self) -> bool:
        return 200 <= self.status < 300

    @property
    def is_error(self) -> bool:
        return not self.is_success


class Slice:
    """Filtered, queryable view over response records.

    All computed values are properties (lazy). Returns 0/0.0 for empty
    slices on numeric properties. Assertion methods fail on empty slices.
    """

    def __init__(self, records: Sequence[ResponseRecord]) -> None:
        self._records = list(records)

    def __len__(self) -> int:
        return len(self._records)

    def __iter__(self):
        return iter(self._records)

    @property
    def count(self) -> int:
        return len(self._records)

    @property
    def error_count(self) -> int:
        return sum(1 for r in self._records if r.is_error)

    @property
    def success_count(self) -> int:
        return sum(1 for r in self._records if r.is_success)

    @property
    def error_rate(self) -> float:
        if not self._records:
            return 0.0
        return self.error_count / len(self._records)

    @property
    def achieved_rate(self) -> float:
        """Requests per second actually delivered."""
        if len(self._records) < 2:
            return 0.0
        duration = self._records[-1].timestamp - self._records[0].timestamp
        if duration <= 0:
            return 0.0
        return len(self._records) / duration

    @property
    def status_codes(self) -> Counter:
        return Counter(r.status for r in self._records)

    @property
    def latency_p50(self) -> float:
        return self.latency_percentile(0.50)

    @property
    def latency_p95(self) -> float:
        return self.latency_percentile(0.95)

    @property
    def latency_p99(self) -> float:
        return self.latency_percentile(0.99)

    def latency_percentile(self, p: float) -> float:
        if not self._records:
            return 0.0
        latencies = sorted(r.latency for r in self._records)
        idx = int(len(latencies) * p)
        idx = min(idx, len(latencies) - 1)
        return latencies[idx]

    def where(self, predicate: Callable[[ResponseRecord], bool]) -> Slice:
        return Slice([r for r in self._records if predicate(r)])

    def errors(self) -> Slice:
        return self.where(lambda r: r.is_error)

    def successes(self) -> Slice:
        return self.where(lambda r: r.is_success)

    def by(self, classifier: Callable[[ResponseRecord], str]) -> Dict[str, Slice]:
        groups: Dict[str, List[ResponseRecord]] = {}
        for r in self._records:
            key = classifier(r)
            groups.setdefault(key, []).append(r)
        return {k: Slice(v) for k, v in groups.items()}

    def by_header(self, name: str, *, missing: str = "<missing>") -> Dict[str, Slice]:
        return self.by(lambda r: r.headers.get(name.lower(), missing))

    def by_status(self) -> Dict[int, Slice]:
        groups: Dict[int, List[ResponseRecord]] = {}
        for r in self._records:
            groups.setdefault(r.status, []).append(r)
        return {k: Slice(v) for k, v in groups.items()}

    def header_values(self, name: str) -> Counter:
        return Counter(r.headers.get(name.lower(), "") for r in self._records)

    def first_seen(
        self, predicate: Callable[[ResponseRecord], bool]
    ) -> Optional[ResponseRecord]:
        for r in self._records:
            if predicate(r):
                return r
        return None

    def assert_error_rate(self, max_rate: float, msg: str = "") -> None:
        if not self._records:
            raise AssertionError(
                f"{msg + ': ' if msg else ''}cannot assert error rate on empty slice"
            )
        if self.error_rate > max_rate:
            raise AssertionError(
                f"{msg + ': ' if msg else ''}"
                f"error rate {self.error_rate:.1%} exceeds {max_rate:.1%}\n"
                f"{self.summary()}"
            )

    def assert_no_errors(self, msg: str = "") -> None:
        self.assert_error_rate(0.0, msg)

    def assert_min_samples(self, n: int, msg: str = "") -> None:
        if self.count < n:
            raise AssertionError(
                f"{msg + ': ' if msg else ''}"
                f"expected at least {n} samples, got {self.count}\n"
                f"{self.summary()}"
            )

    def summary(self) -> str:
        if not self._records:
            return "Slice: 0 requests"
        duration = self._records[-1].timestamp - self._records[0].timestamp
        lines = [
            f"Slice: {self.count} requests over {duration:.1f}s "
            f"({self.achieved_rate:.1f} req/s)",
            f"  Successes: {self.success_count} ({self.success_count * 100 / self.count:.1f}%)",
            f"  Errors:    {self.error_count} ({self.error_rate:.1%})",
            f"  Status codes: {dict(self.status_codes)}",
        ]
        if self.success_count > 0:
            lines.append(
                f"  Latency p50: {self.latency_p50:.3f}s, "
                f"p95: {self.latency_p95:.3f}s, "
                f"p99: {self.latency_p99:.3f}s"
            )
        if self.error_count > 0:
            samples = []
            for r in self._records:
                if r.is_error and (r.body or r.error):
                    samples.append(f"    [{r.status}] {r.body or r.error}")
                    if len(samples) >= 3:
                        break
            if samples:
                lines.append("  Error samples:")
                lines.extend(samples)
        return "\n".join(lines)


class TrafficReport:
    """Complete record of a traffic driving session."""

    def __init__(
        self,
        records: List[ResponseRecord],
        marks: Dict[str, float],
        stop_timestamp: float,
    ) -> None:
        self._records = sorted(records, key=lambda r: r.timestamp)
        self._marks = dict(marks)
        self._stop_ts = stop_timestamp
        self._mark_order = sorted(marks.items(), key=lambda x: x[1])

    @property
    def all(self) -> Slice:
        return Slice(self._records)

    @property
    def marks(self) -> Dict[str, float]:
        return dict(self._marks)

    @property
    def duration(self) -> float:
        if not self._records:
            return 0.0
        return self._records[-1].timestamp - self._records[0].timestamp

    def phase(self, name: str) -> Slice:
        """Records between mark `name` and the next mark (half-open)."""
        if name not in self._marks:
            available = [m[0] for m in self._mark_order]
            raise KeyError(f"Unknown mark {name!r}. Available: {available}")
        start = self._marks[name]
        end = self._stop_ts
        for i, (mark_name, _mark_ts) in enumerate(self._mark_order):
            if mark_name == name and i + 1 < len(self._mark_order):
                end = self._mark_order[i + 1][1]
                break
        return Slice([r for r in self._records if start <= r.timestamp < end])

    def between(self, a: str, b: str) -> Slice:
        """Half-open interval [mark_a, mark_b)."""
        for name in (a, b):
            if name not in self._marks:
                available = [m[0] for m in self._mark_order]
                raise KeyError(f"Unknown mark {name!r}. Available: {available}")
        start = self._marks[a]
        end = self._marks[b]
        return Slice([r for r in self._records if start <= r.timestamp < end])

    def around(self, name: str, *, before: float = 5.0, after: float = 5.0) -> Slice:
        """Closed interval [mark - before, mark + after]."""
        if name not in self._marks:
            available = [m[0] for m in self._mark_order]
            raise KeyError(f"Unknown mark {name!r}. Available: {available}")
        ts = self._marks[name]
        return Slice(
            [r for r in self._records if ts - before <= r.timestamp <= ts + after]
        )

    def since(self, name: str) -> Slice:
        """[mark, end_of_recording)."""
        if name not in self._marks:
            available = [m[0] for m in self._mark_order]
            raise KeyError(f"Unknown mark {name!r}. Available: {available}")
        start = self._marks[name]
        return Slice([r for r in self._records if r.timestamp >= start])

    def until(self, name: str) -> Slice:
        """[start_of_recording, mark)."""
        if name not in self._marks:
            available = [m[0] for m in self._mark_order]
            raise KeyError(f"Unknown mark {name!r}. Available: {available}")
        end = self._marks[name]
        return Slice([r for r in self._records if r.timestamp < end])
