#!/bin/bash

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

# Runs a focused subset of the Go tests for `make test-focus`, which provides
# KUBEBUILDER_ASSETS, GOFLAGS (build tags) and TEST_TIMEOUT.
#
#   PKG    go package patterns, space-separated (required)
#   RUN    `go test -run` regex
#   FOCUS  Ginkgo focus regex
#
# Fails when nothing ran. A `-run` that matches nothing and a Ginkgo focus
# that matches no spec both report `ok` from `go test`.

set -o errexit -o nounset -o pipefail

PKG=${PKG:-}
RUN=${RUN:-}
FOCUS=${FOCUS:-}

read -ra pkgs <<<"${PKG}"

if ((${#pkgs[@]} == 0)); then
  echo "usage: make test-focus PKG=<package pattern> [FOCUS=<ginkgo focus regex>] [RUN=<go test -run regex>]" >&2
  exit 2
fi

if [[ -n ${FOCUS} ]]; then
  # A test binary that does not link Ginkgo rejects -ginkgo.* flags, so a
  # pattern like ./pkg/... is narrowed to the packages with a Ginkgo suite.
  listing=$(go list -f '{{.ImportPath}} {{join .TestImports " "}} {{join .XTestImports " "}}' "${pkgs[@]}")
  suites=()
  while read -r path imports; do
    if [[ " ${imports} " == *" github.com/onsi/ginkgo/v2 "* ]]; then
      suites+=("${path}")
    fi
  done <<<"${listing}"
  if ((${#suites[@]} == 0)); then
    echo "test-focus: FOCUS selects Ginkgo specs, and no package in '${PKG}' has a Ginkgo suite" >&2
    exit 1
  fi
  skipped=$(($(wc -l <<<"${listing}") - ${#suites[@]}))
  if ((skipped > 0)); then
    echo "test-focus: FOCUS runs Ginkgo suites only, skipping ${skipped} package(s) without one"
  fi
  pkgs=("${suites[@]}")
fi

args=(-count=1 -v -timeout "${TEST_TIMEOUT:-30m}")
if [[ -n ${RUN} ]]; then
  args+=(-run "${RUN}")
fi
args+=("${pkgs[@]}")
if [[ -n ${FOCUS} ]]; then
  args+=(-args -ginkgo.v -ginkgo.focus="${FOCUS}")
fi

printf 'GOFLAGS=%q go test' "${GOFLAGS:-}"
printf ' %q' "${args[@]}"
printf '\n'

log=$(mktemp)
trap 'rm -f "${log}"' EXIT

go test "${args[@]}" | tee "${log}"

if [[ -n ${FOCUS} ]]; then
  # Ginkgo prints `Ran <passed+failed> of <total> Specs` once per suite.
  specs=$(awk 'match($0, /Ran [0-9]+ of [0-9]+ Specs/) { split(substr($0, RSTART, RLENGTH), f, " "); n += f[2] } END { print n + 0 }' "${log}")
  if ((specs == 0)); then
    echo "test-focus: no Ginkgo spec ran: FOCUS matched none, or RUN skipped the suite's Test function" >&2
    exit 1
  fi
elif ! awk '/^ok[ \t]/ && !/\[no tests to run\]/ { ran = 1 } END { exit !ran }' "${log}"; then
  # `=== RUN` is not enough: a subtest pattern that matches nothing still runs
  # the parent test, and go test then marks the package `[no tests to run]`.
  echo "test-focus: no test ran: PKG has no tests, or RUN matched none" >&2
  exit 1
fi
