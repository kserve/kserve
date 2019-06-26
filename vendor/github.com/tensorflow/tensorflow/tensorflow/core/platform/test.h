/* Copyright 2015 The TensorFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
==============================================================================*/

#ifndef TENSORFLOW_CORE_PLATFORM_TEST_H_
#define TENSORFLOW_CORE_PLATFORM_TEST_H_

#include <memory>
#include <vector>

#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/platform.h"
#include "tensorflow/core/platform/types.h"

// As of September 2016, we continue to attempt to avoid the use of gmock aka
// googlemock included in the test framework
// (https://github.com/google/googletest) to discourage over-eager use of mocks
// that lead to cumbersome class hierarchies and tests that might end up not
// testing real code in important ways.
#if defined(PLATFORM_GOOGLE) || defined(PLATFORM_GOOGLE_ANDROID)
#include "tensorflow/core/platform/google/build_config/gunit.h"
#else
#include <gtest/gtest.h>
#endif

namespace tensorflow {
namespace testing {

// Return a temporary directory suitable for temporary testing files.
string TmpDir();

// Returns the path to TensorFlow in the directory containing data
// dependencies.
string TensorFlowSrcRoot();

// Return a random number generator seed to use in randomized tests.
// Returns the same value for the lifetime of the process.
int RandomSeed();

// Returns an unused port number, for use in multi-process testing.
// NOTE: This function is not thread-safe.
int PickUnusedPortOrDie();

}  // namespace testing
}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_PLATFORM_TEST_H_
