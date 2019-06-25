/* Copyright 2018 The TensorFlow Authors. All Rights Reserved.

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

#ifndef TENSORFLOW_JAVA_SRC_MAIN_NATIVE_UTILS_JNI_H_
#define TENSORFLOW_JAVA_SRC_MAIN_NATIVE_UTILS_JNI_H_

#include <jni.h>

#include "tensorflow/c/c_api.h"

#ifdef __cplusplus
extern "C" {
#endif  // __cplusplus

void resolveOutputs(JNIEnv* env, const char* type, jlongArray src_op,
                    jintArray src_index, TF_Output* dst, jint n);

#ifdef __cplusplus
}  // extern "C"
#endif  // __cplusplus
#endif  // TENSORFLOW_JAVA_SRC_MAIN_NATIVE_UTILS_JNI_H_
