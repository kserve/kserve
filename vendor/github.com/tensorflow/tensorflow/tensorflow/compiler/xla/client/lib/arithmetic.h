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

#ifndef TENSORFLOW_COMPILER_XLA_CLIENT_LIB_ARITHMETIC_H_
#define TENSORFLOW_COMPILER_XLA_CLIENT_LIB_ARITHMETIC_H_

#include <memory>

#include "tensorflow/compiler/xla/client/xla_builder.h"
#include "tensorflow/compiler/xla/client/xla_computation.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"

namespace xla {

// Creates a scalar add computation and returns it.
XlaComputation CreateScalarAddComputation(PrimitiveType type,
                                          XlaBuilder* builder);

// Creates a scalar multiply computation and returns it.
XlaComputation CreateScalarMultiplyComputation(PrimitiveType type,
                                               XlaBuilder* builder);

// Creates a scalar ge computation and returns it.
XlaComputation CreateScalarGeComputation(PrimitiveType type,
                                         XlaBuilder* builder);

// Creates a scalar max computation and returns it.
XlaComputation CreateScalarMaxComputation(PrimitiveType type,
                                          XlaBuilder* builder);

// Creates a scalar min computation and returns it.
XlaComputation CreateScalarMinComputation(PrimitiveType type,
                                          XlaBuilder* builder);

// Creates a scalar logical AND computation and returns it.
XlaComputation CreateScalarAndComputation(PrimitiveType type,
                                          XlaBuilder* builder);

// Creates a scalar logical OR computation and returns it.
XlaComputation CreateScalarOrComputation(PrimitiveType type,
                                         XlaBuilder* builder);

// Returns whether any predicate in "predicates" is set.
//
// Note: if predicates is zero-sized, Any() vacuously returns false.
XlaOp Any(XlaOp predicates);

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_CLIENT_LIB_ARITHMETIC_H_
