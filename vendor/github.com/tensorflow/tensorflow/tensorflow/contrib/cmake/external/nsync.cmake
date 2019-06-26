# Copyright 2017 The TensorFlow Authors. All Rights Reserved.
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
include (ExternalProject)

set(nsync_INCLUDE_DIR ${CMAKE_CURRENT_BINARY_DIR}/external/nsync/public)
set(nsync_URL https://github.com/google/nsync)
set(nsync_TAG 1.20.1)
set(nsync_BUILD ${CMAKE_CURRENT_BINARY_DIR}/nsync/src/nsync)
set(nsync_INSTALL ${CMAKE_CURRENT_BINARY_DIR}/nsync/install)

if(WIN32)
  set(nsync_HEADERS "${nsync_BUILD}/public/*.h")
  set(nsync_STATIC_LIBRARIES ${nsync_INSTALL}/lib/nsync_cpp.lib)
else()
  set(nsync_HEADERS "${nsync_BUILD}/public/*.h")
  set(nsync_STATIC_LIBRARIES ${nsync_INSTALL}/lib/libnsync_cpp.a)
endif()

ExternalProject_Add(nsync
    PREFIX nsync
    GIT_REPOSITORY ${nsync_URL}
    GIT_TAG ${nsync_TAG}
    DOWNLOAD_DIR "${DOWNLOAD_LOCATION}"
    BUILD_IN_SOURCE 1
    BUILD_BYPRODUCTS ${nsync_STATIC_LIBRARIES}
    INSTALL_DIR ${nsync_INSTALL}
    CMAKE_CACHE_ARGS
        -DCMAKE_BUILD_TYPE:STRING=Release
        -DCMAKE_VERBOSE_MAKEFILE:BOOL=OFF
        -DCMAKE_INSTALL_PREFIX:STRING=${nsync_INSTALL}
        -DCMAKE_INSTALL_LIBDIR:STRING=lib
    -DNSYNC_LANGUAGE:STRING=c++11)

set(nsync_HEADERS
    "${nsync_INSTALL}/include/nsync.h"
    "${nsync_INSTALL}/include/nsync_atomic.h"
    "${nsync_INSTALL}/include/nsync_counter.h"
    "${nsync_INSTALL}/include/nsync_cpp.h"
    "${nsync_INSTALL}/include/nsync_cv.h"
    "${nsync_INSTALL}/include/nsync_debug.h"
    "${nsync_INSTALL}/include/nsync_mu.h"
    "${nsync_INSTALL}/include/nsync_mu_wait.h"
    "${nsync_INSTALL}/include/nsync_note.h"
    "${nsync_INSTALL}/include/nsync_once.h"
    "${nsync_INSTALL}/include/nsync_time.h"
    "${nsync_INSTALL}/include/nsync_time_internal.h"
    "${nsync_INSTALL}/include/nsync_waiter.h"
)
                                                            
# put nsync includes in the directory where they are expected
add_custom_target(nsync_create_destination_dir
    COMMAND ${CMAKE_COMMAND} -E make_directory ${nsync_INCLUDE_DIR}
    DEPENDS nsync)

add_custom_target(nsync_copy_headers_to_destination
    DEPENDS nsync_create_destination_dir)

foreach(header_file ${nsync_HEADERS})
  add_custom_command(TARGET nsync_copy_headers_to_destination PRE_BUILD
      COMMAND ${CMAKE_COMMAND} -E copy_if_different ${header_file} ${nsync_INCLUDE_DIR}/)
endforeach()


