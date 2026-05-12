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
from datetime import datetime
from kubernetes import client, config, dynamic
from kubernetes.client import api_client
from typing import Optional


def print_all_events_table(namespace: str, max_events: int = 50):
    """
    Print the most recent `max_events` events in `namespace` as a nice table.
    """
    core = client.CoreV1Api()

    try:
        events = core.list_namespaced_event(namespace=namespace).items

        if not events:
            print("ℹ️ # No events found in namespace", namespace)
            return

        header = f"{'TIME':<25} {'NAMESPACE':<12} {'SOURCE':<20} {'TYPE':<8} {'REASON':<20} MESSAGE"
        print(header)
        print("-" * len(header))

        for ev in events:
            ts = ev.last_timestamp or ev.first_timestamp
            ts_str = (
                ts.strftime("%Y-%m-%d %H:%M:%S")
                if isinstance(ts, datetime)
                else str(ts)
            )
            src = f"{ev.source.component or ''}/{ev.source.host or ''}".strip("/")
            msg = (ev.message or "").replace("\n", " ")
            print(
                f"{ts_str:<25} {ev.metadata.namespace:<12} {src:<20} {ev.type or '':<8} "
                f"{ev.reason or '':<20} {msg}"
            )

    except Exception as e:
        print(f"# ❌ failed to list events: {e}")


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
            print(
                f"failed to check resource properties for {getattr(rsrc, 'kind', 'unknown')}, skipping: {e}"
            )
            continue

        try:
            resp = rsrc.get(namespace=namespace, label_selector=selector)
        except Exception as e:
            print(f"failed to get {rsrc.kind}, skipping: {e}")
            continue

        items = getattr(resp, "items", [])
        found.extend(items)

    return found


def collect_pod_logs(namespace: str, labels):
    """
    For every pod in `namespace` matching `labels`, print logs for all init
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
        print(f"# failed to list pods: {e}")
        return

    if not pods:
        print(f"# no pods found in {namespace} matching {selector}")
        return

    for pod in pods:
        pod_name = pod.metadata.name
        phase = pod.status.phase if pod.status else "Unknown"
        print(f"\n### Pod {pod_name} (phase={phase})")

        init_specs = pod.spec.init_containers or []
        init_statuses = {
            s.name: s for s in (pod.status.init_container_statuses or [])
        }
        for c in init_specs:
            _print_container_logs(
                core, namespace, pod_name, c.name,
                is_init=True, status=init_statuses.get(c.name),
            )

        c_specs = pod.spec.containers or []
        c_statuses = {
            s.name: s for s in (pod.status.container_statuses or [])
        }
        for c in c_specs:
            _print_container_logs(
                core, namespace, pod_name, c.name,
                is_init=False, status=c_statuses.get(c.name),
            )


def _print_container_logs(
    core: client.CoreV1Api,
    namespace: str,
    pod_name: str,
    container_name: str,
    is_init: bool,
    status: Optional[object],
):
    kind = "init-container" if is_init else "container"
    restart_count = status.restart_count if status else 0
    print(f"\n#### {kind} {container_name!r} (restarts={restart_count})")

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
            print(f"# -- logs ({label}) --")
            print(logs or "(empty)")
        except Exception as e:
            print(f"# -- logs ({label}): unavailable ({e})")
