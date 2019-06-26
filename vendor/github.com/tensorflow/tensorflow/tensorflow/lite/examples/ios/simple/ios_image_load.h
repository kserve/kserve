// Copyright 2015 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#ifndef TENSORFLOW_LITE_EXAMPLES_IOS_SIMPLE_IOS_IMAGE_LOAD_H_
#define TENSORFLOW_LITE_EXAMPLES_IOS_SIMPLE_IOS_IMAGE_LOAD_H_

#include <vector>

std::vector<uint8_t> LoadImageFromFile(const char* file_name, int* out_width,
                                       int* out_height, int* out_channels);

#endif  // TENSORFLOW_LITE_EXAMPLES_IOS_SIMPLE_IOS_IMAGE_LOAD_H_
