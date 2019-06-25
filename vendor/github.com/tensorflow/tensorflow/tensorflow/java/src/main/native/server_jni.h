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

#ifndef TENSORFLOW_JAVA_SRC_MAIN_NATIVE_SERVER_JNI_H_
#define TENSORFLOW_JAVA_SRC_MAIN_NATIVE_SERVER_JNI_H_

#include <jni.h>

#ifdef __cplusplus
extern "C" {
#endif

/*
 * Class:     org_tensorflow_Server
 * Method:    allocate
 * Signature: ([B)J
 */
JNIEXPORT jlong JNICALL
Java_org_tensorflow_Server_allocate(JNIEnv *, jclass, jbyteArray server_def);

/*
 * Class:     org_tensorflow_Server
 * Method:    start
 * Signature: (J)V
 */
JNIEXPORT void JNICALL Java_org_tensorflow_Server_start(JNIEnv *, jclass,
                                                        jlong);

/*
 * Class:     org_tensorflow_Server
 * Method:    stop
 * Signature: (J)V
 */
JNIEXPORT void JNICALL Java_org_tensorflow_Server_stop(JNIEnv *, jclass, jlong);

/*
 * Class:     org_tensorflow_Session
 * Method:    join
 * Signature: (J)V
 */
JNIEXPORT void JNICALL Java_org_tensorflow_Server_join(JNIEnv *, jclass, jlong);

/*
 * Class:     org_tensorflow_Session
 * Method:    delete
 * Signature: (J)V
 */
JNIEXPORT void JNICALL Java_org_tensorflow_Server_delete(JNIEnv *, jclass,
                                                         jlong);

#ifdef __cplusplus
}  // extern "C"
#endif  // __cplusplus
#endif  // TENSORFLOW_JAVA_SRC_MAIN_NATIVE_SERVER_JNI_H_
