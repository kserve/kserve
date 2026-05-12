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

import itertools
import logging
from datetime import datetime
from kubernetes import client, config, dynamic
from kubernetes.client import api_client
from typing import Callable, Optional

_log = logging.getLogger(__name__)


def print_all_events_table(
    namespace: str, max_events: int = 50, log: Callable = _log.info
):
    """
    Emit the most recent `max_events` events in `namespace` as a nice table.
    """
    core = client.CoreV1Api()

    try:
        events = core.list_namespaced_event(namespace=namespace).items

        if not events:
            log("ℹ️ # No events found in namespace %s", namespace)
            return

        header = f"{'TIME':<25} {'NAMESPACE':<12} {'SOURCE':<20} {'TYPE':<8} {'REASON':<20} MESSAGE"
        log(header)
        log("-" * len(header))

        for ev in events:
            ts = ev.last_timestamp or ev.first_timestamp
            ts_str = (
                ts.strftime("%Y-%m-%d %H:%M:%S")
                if isinstance(ts, datetime)
                else str(ts)
            )
            src = f"{ev.source.component or ''}/{ev.source.host or ''}".strip("/")
            msg = (ev.message or "").replace("\n", " ")
            log(
                "%s %s %s %s %s %s",
                ts_str.ljust(25),
                ev.metadata.namespace.ljust(12),
                src.ljust(20),
                (ev.type or "").ljust(8),
                (ev.reason or "").ljust(20),
                msg,
            )

    except Exception as e:
        log("# ❌ failed to list events: %s", e)


def kinds_matching_by_labels(namespace: str, labels, skip_api_kinds=None):
    """
    List all namespaced objects in `namespace` matching `labels`
    whose kinds are not in `skip_api_kinds`.

    :param namespace: Namespace to search
    :param labels: either a dict of {k: v} or a raw selector string
    :param skip_api_kinds: an iterable of Resource.kind strings to exclude
    :return: a list of Unstructured objects
    """
    if skip_api_kinds is None:
        skip_api_kinds = {"Secret"}

    config.load_kube_config()
    dyn = dynamic.DynamicClient(api_client.ApiClient())

    selector = (
        ",".join(f"{k}={v}" for k, v in labels.items())
        if isinstance(labels, dict)
        else labels
    )

    all_resources = itertools.chain.from_iterable(dyn.resources)

    found = []
    for rsrc in all_resources:
        if rsrc.kind.endswith("List") or rsrc.kind in skip_api_kinds:
            continue

        try:
            if not rsrc.namespaced or "list" not in rsrc.verbs:
                continue
        except Exception as e:
            _log.debug(
                "failed to check resource properties for %s, skipping: %s",
                getattr(rsrc, "kind", "unknown"),
                e,
            )
            continue

        try:
            resp = rsrc.get(namespace=namespace, label_selector=selector)
        except Exception as e:
            _log.debug("failed to get %s, skipping: %s", rsrc.kind, e)
            continue

        items = getattr(resp, "items", [])
        found.extend(items)

    return found


def collect_pod_logs(namespace: str, labels, log: Callable = _log.info):
    """
    For every pod in `namespace` matching `labels`, emit logs for all init
    containers and regular containers (current and, when restarted, previous).
    """
    core = client.CoreV1Api()
    selector = (
        ",".join(f"{k}={v}" for k, v in labels.items())
        if isinstance(labels, dict)
        else labels
    )
    try:
        pods = core.list_namespaced_pod(
            namespace=namespace, label_selector=selector
        ).items
    except Exception as e:
        log("# failed to list pods: %s", e)
        return

    if not pods:
        log("# no pods found in %s matching %s", namespace, selector)
        return

    for pod in pods:
        pod_name = pod.metadata.name
        phase = pod.status.phase if pod.status else "Unknown"
        log("### Pod %s (phase=%s)", pod_name, phase)

        init_specs = pod.spec.init_containers or []
        init_statuses = {
            s.name: s for s in (pod.status.init_container_statuses or [])
        }
        for c in init_specs:
            _emit_container_logs(
                core, namespace, pod_name, c.name,
                is_init=True, status=init_statuses.get(c.name), log=log,
            )

        c_specs = pod.spec.containers or []
        c_statuses = {
            s.name: s for s in (pod.status.container_statuses or [])
        }
        for c in c_specs:
            _emit_container_logs(
                core, namespace, pod_name, c.name,
                is_init=False, status=c_statuses.get(c.name), log=log,
            )


def _emit_container_logs(
    core: client.CoreV1Api,
    namespace: str,
    pod_name: str,
    container_name: str,
    is_init: bool,
    status: Optional[object],
    log: Callable = _log.info,
):
    kind = "init-container" if is_init else "container"
    restart_count = status.restart_count if status else 0
    log("#### %s %r (restarts=%d)", kind, container_name, restart_count)

    revisions = [False, True] if restart_count > 0 else [False]
    for previous in revisions:
        label = "previous" if previous else "current"
        try:
            logs = core.read_namespaced_pod_log(
                name=pod_name,
                namespace=namespace,
                container=container_name,
                previous=previous,
                tail_lines=200,
            )
            log("# -- logs (%s) --", label)
            log("%s", logs or "(empty)")
        except Exception as e:
            log("# -- logs (%s): unavailable (%s)", label, e)
