#!/usr/bin/env bash
# Copyright 2015 The TensorFlow Authors. All Rights Reserved.
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
# ==============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/builds_common.sh"
configure_android_workspace

# The Bazel Android demo and Makefile builds are intentionally built for x86_64
# and armeabi-v7a respectively to maximize build coverage while minimizing
# compilation time. For full build coverage and exposed binaries, see
# android_full.sh

echo "========== TensorFlow Demo Build Test =========="
TARGETS=
TARGETS+=" //tensorflow/examples/android:tensorflow_demo"
# Also build the Eager Runtime so it remains compatible with Android for the
# benefits of clients like TensorFlow Lite. For now it is enough to build only
# :execute, which what TF Lite needs.
TARGETS+=" //tensorflow/core/common_runtime/eager:execute"
# Enable sandboxing so that zip archives don't get incorrectly packaged
# in assets/ dir (see https://github.com/bazelbuild/bazel/issues/2334)
# TODO(gunan): remove extra flags once sandboxing is enabled for all builds.
bazel --bazelrc=/dev/null build \
    --compilation_mode=opt --cxxopt=-std=c++11 --fat_apk_cpu=x86_64 \
    --spawn_strategy=sandboxed --genrule_strategy=sandboxed \
    --define=grpc_no_ares=true \
    ${TARGETS}

echo "========== Makefile Build Test =========="
# Test Makefile build just to make sure it still works.
if [ -z "$NDK_ROOT" ]; then
   export NDK_ROOT=${ANDROID_NDK_HOME}
fi
tensorflow/contrib/makefile/build_all_android.sh
