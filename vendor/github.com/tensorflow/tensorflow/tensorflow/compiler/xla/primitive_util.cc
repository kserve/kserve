/* Copyright 2017 The TensorFlow Authors. All Rights Reserved.

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

#include "tensorflow/compiler/xla/primitive_util.h"

#include "tensorflow/core/platform/logging.h"

namespace xla {
namespace primitive_util {

bool IsFloatingPointType(PrimitiveType type) {
  return type == F16 || type == F32 || type == F64 || type == BF16;
}

bool IsComplexType(PrimitiveType type) { return type == C64; }

bool IsSignedIntegralType(PrimitiveType type) {
  return type == S8 || type == S16 || type == S32 || type == S64;
}

bool IsUnsignedIntegralType(PrimitiveType type) {
  return type == U8 || type == U16 || type == U32 || type == U64;
}

bool IsIntegralType(PrimitiveType type) {
  return IsUnsignedIntegralType(type) || IsSignedIntegralType(type);
}

int BitWidth(PrimitiveType type) {
  switch (type) {
    case PRED:
      return 1;

    case S8:
    case U8:
      return 8;

    case S16:
    case U16:
    case F16:
    case BF16:
      return 16;

    case U32:
    case S32:
    case F32:
      return 32;

    case U64:
    case S64:
    case F64:
    case C64:
      return 64;

    case TUPLE:
      LOG(FATAL) << "TUPLE is an invalid type for BitWidth";

    case OPAQUE:
      LOG(FATAL) << "OPAQUE is an invalid type for BitWidth";

    default:
      LOG(FATAL) << "Unhandled primitive type " << type;
  }
}

PrimitiveType ComplexComponentType(PrimitiveType complex_type) {
  switch (complex_type) {
    case C64:
      return F32;
    default:
      LOG(FATAL) << "Primitive type is not complex: "
                 << PrimitiveType_Name(complex_type);
  }
}

bool IsArrayType(PrimitiveType primitive_type) {
  return primitive_type != PRIMITIVE_TYPE_INVALID && primitive_type != TUPLE &&
         primitive_type != OPAQUE && primitive_type != TOKEN;
}

}  // namespace primitive_util
}  // namespace xla
