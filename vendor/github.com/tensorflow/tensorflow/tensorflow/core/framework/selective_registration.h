/* Copyright 2016 The TensorFlow Authors. All Rights Reserved.

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

#ifndef TENSORFLOW_CORE_FRAMEWORK_SELECTIVE_REGISTRATION_H_
#define TENSORFLOW_CORE_FRAMEWORK_SELECTIVE_REGISTRATION_H_

#include <string.h>

#ifdef SELECTIVE_REGISTRATION

// Experimental selective registration support to reduce binary size.
//
// To use selective registration, when building:
// 1. define SELECTIVE_REGISTRATION, e.g. in gcc by passing
//    -DSELECTIVE_REGISTRATION to compilation.
// 2. Provide ops_to_register.h. This file is not included in the repo and must
//    be placed by the user or a tool where the compiler can find it.  It must
//    define the constants and functions used in the macros below. The
//    functions should be defined as valid constexpr functions, so that they are
//    evaluated at compile time: this is needed to make symbols referenced by
//    un-registered objects unused, and therefore allow the linker to strip them
//    out.  See python/tools/print_selective_registration_header.py for a tool
//    that can be used to generate ops_to_register.h.
//
// ops_to_register.h should define macros for:
//   // Ops for which this is false will not be registered.
//   SHOULD_REGISTER_OP(op)
//   // If this is false, then no gradient ops are registered.
//   SHOULD_REGISTER_OP_GRADIENT
//   // Op kernel classes where this is false won't be registered.
//   SHOULD_REGISTER_OP_KERNEL(clz)
// The macros should be defined using constexprs.

#include "ops_to_register.h"

#if (!defined(SHOULD_REGISTER_OP) || !defined(SHOULD_REGISTER_OP_GRADIENT) || \
     !defined(SHOULD_REGISTER_OP_KERNEL))
static_assert(false, "ops_to_register.h must define SHOULD_REGISTER macros");
#endif
#else
#define SHOULD_REGISTER_OP(op) true
#define SHOULD_REGISTER_OP_GRADIENT true
#define SHOULD_REGISTER_OP_KERNEL(clz) true
#endif

#endif  // TENSORFLOW_CORE_FRAMEWORK_SELECTIVE_REGISTRATION_H_
