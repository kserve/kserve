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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_HLO_EVALUATOR_TYPED_VISITOR_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_HLO_EVALUATOR_TYPED_VISITOR_H_

#include <cmath>

#include "absl/algorithm/container.h"
#include "absl/base/casts.h"
#include "absl/container/inlined_vector.h"
#include "absl/memory/memory.h"
#include "absl/types/optional.h"
#include "tensorflow/compiler/xla/array2d.h"
#include "tensorflow/compiler/xla/literal_util.h"
#include "tensorflow/compiler/xla/service/hlo_casting_utils.h"
#include "tensorflow/compiler/xla/service/hlo_evaluator.h"
#include "tensorflow/compiler/xla/service/hlo_instructions.h"
#include "tensorflow/compiler/xla/service/shape_inference.h"

namespace xla {

// TODO(b/79274244): We'd like these type traits to live inside of
// HloEvaluatorTypedVisitor so they don't pollute namespace xla, but that
// crashes clang in the frontend.
//
// Anyway this is relatively safe as-is because hlo_evaluator_typed_visitor.h is
// a "private" header that's not exposed outside of hlo_evaluator.cc.
template <typename T>
using is_complex_t = std::is_same<T, complex64>;
template <typename T>
using is_complex64_t = std::is_same<T, complex64>;

// It's UB to use std::sort with std::less<float>, because of NaNs. Define
// "safe" less functions which are actually strict weak orders. -NaN and NaN
// should appear at the beginning and end of the ordering, and -0.0 should
// appear before 0.0.
template <
    typename NativeT,
    typename std::enable_if<std::is_integral<NativeT>::value>::type* = nullptr>
bool SafeLess(const NativeT& a, const NativeT& b) {
  return a < b;
}

template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                NativeT>::value>::type* = nullptr>
bool SafeLess(const NativeT& a, const NativeT& b) {
  bool lhs_is_negative = std::signbit(a);
  bool rhs_is_negative = std::signbit(b);
  // If the signs are different, we can just compare the signs.
  if (lhs_is_negative != rhs_is_negative) {
    return lhs_is_negative && !rhs_is_negative;
  }
  bool lhs_nan = std::isnan(a);
  bool rhs_nan = std::isnan(b);
  // Exactly one number is nan?
  if (lhs_nan != rhs_nan) {
    if (lhs_nan) {
      return lhs_is_negative;
    }
    return !rhs_is_negative;
  }
  return a < b;
}

template <typename NativeT,
          typename std::enable_if<
              std::is_same<NativeT, bfloat16>::value ||
              std::is_same<NativeT, Eigen::half>::value>::type* = nullptr>
bool SafeLess(const NativeT& a, const NativeT& b) {
  return SafeLess(static_cast<float>(a), static_cast<float>(b));
}

// Templated DfsHloVisitor for use by HloEvaluator.
//
// Typically ReturnT here indicates the resulting literal type of each evaluated
// Handle* method of a TypedVisitor.  There are however a few notable exceptions
// to this rule, notably:
// - HandleCompare and HandleIsFinite: where the resulting literal type is
//   always boolean.
// - HandleImag and HandleReal: where the resulting literal type is always float
//   and the operand is always complex, or real in the case of HandleReal.
// These operations are handled outside of the parent HloEvaluator handlers
// instead of from within TypedVisitor.
//
// Type params:
//   - ReturnT: The type of input and output of each operation.
//   - ElementwiseT: The type in which internal computation are done.
//
// This a logically a private part of HloEvaluator.  It lives in this header
// file rather than in hlo_evaluator.cc because we use extern templates and a
// bunch of independent cc files to speed up compiling the many instantiations
// of this class.
template <typename ReturnT, typename ElementwiseT = ReturnT>
class HloEvaluatorTypedVisitor : public DfsHloVisitorWithDefault {
 private:
  Status UnsupportedTypeError(HloInstruction* instruction) {
    return InvalidArgument(
        "Unsupported type for %s: %s", HloOpcodeString(instruction->opcode()),
        PrimitiveType_Name(instruction->shape().element_type()));
  }

  // Get the value in the given literal static_cast as a double.
  template <
      typename NativeT,
      typename std::enable_if<!is_complex_t<NativeT>::value>::type* = nullptr>
  double GetAsDouble(const Literal& literal,
                     absl::Span<const int64> input_index) {
    return static_cast<double>(literal.Get<NativeT>(input_index));
  }

  // Specialization for complex types. In this case it is not possible to
  // static_cast value to a double so just CHECK fail. This method is not used
  // at run-time, but must be available at compile-time to keep the compiler
  // happy.
  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  double GetAsDouble(const Literal& literal,
                     absl::Span<const int64> input_index) {
    LOG(FATAL) << "Trying to get complex literal as double: "
               << literal.ToString();
  }

 public:
  explicit HloEvaluatorTypedVisitor(HloEvaluator* p) : parent_(p) {}

  // The following higher-order functions convert a function with ElementwiseT
  // to a function with ReturnT.
  std::function<ReturnT(ReturnT)> ConvertUnaryFunction(
      const std::function<ElementwiseT(ElementwiseT)>& unary_op) {
    return [&unary_op](ReturnT arg) {
      return static_cast<ReturnT>(unary_op(static_cast<ElementwiseT>(arg)));
    };
  }
  std::function<ReturnT(ReturnT, ReturnT)> ConvertBinaryFunction(
      const std::function<ElementwiseT(ElementwiseT, ElementwiseT)>&
          binary_op) {
    return [&binary_op](ReturnT arg1, ReturnT arg2) {
      return static_cast<ReturnT>(binary_op(static_cast<ElementwiseT>(arg1),
                                            static_cast<ElementwiseT>(arg2)));
    };
  }
  std::function<ReturnT(ReturnT, ReturnT, ReturnT)> ConvertTernaryFunction(
      const std::function<ElementwiseT(ElementwiseT, ElementwiseT,
                                       ElementwiseT)>& ternary_op) {
    return [&ternary_op](ReturnT arg1, ReturnT arg2, ReturnT arg3) {
      return static_cast<ReturnT>(ternary_op(static_cast<ElementwiseT>(arg1),
                                             static_cast<ElementwiseT>(arg2),
                                             static_cast<ElementwiseT>(arg3)));
    };
  }

  Status DefaultAction(HloInstruction* hlo_instruction) override {
    return Unimplemented("unhandled HLO ops for HloEvaluator: %s.",
                         HloOpcodeString(hlo_instruction->opcode()));
  }

  template <typename NativeT,
            typename std::enable_if<std::is_unsigned<NativeT>::value>::type* =
                nullptr>
  Status HandleAbs(HloInstruction* abs) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[abs],
                        ElementWiseUnaryOp(abs, [](NativeT elem_operand) {
                          return elem_operand;
                        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<std::is_signed<NativeT>::value>::type* = nullptr>
  Status HandleAbs(HloInstruction* abs) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[abs],
                        ElementWiseUnaryOp(abs, [](NativeT elem_operand) {
                          return std::abs(elem_operand);
                        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex64_t<NativeT>::value>::type* = nullptr>
  Status HandleAbs(HloInstruction* abs) {
    const Literal& operand_literal =
        parent_->GetEvaluatedLiteralFor(abs->operand(0));
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[abs],
        (HloEvaluator::ElementWiseUnaryOpImpl<float, NativeT>(
            abs, [](NativeT elem_operand) { return std::abs(elem_operand); },
            operand_literal)));

    return Status::OK();
  }

  Status HandleAbs(HloInstruction* abs) override {
    // If the operand is of C64 type, the return type of abs will be F32.
    // However, ElementwiseT would still be the return type, F32, and thus
    // specifying the ElementwiseT explicitly as C64 is needed below.
    if (abs->operand(0)->shape().element_type() == C64) {
      return HandleAbs<complex64>(abs);
    }
    return HandleAbs<ElementwiseT>(abs);
  }

  template <
      typename NativeT,
      typename std::enable_if<!is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleRound(HloInstruction* round) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[round],
        ElementWiseUnaryOp(round, [](ElementwiseT elem_operand) {
          return std::round(elem_operand);
        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleRound(HloInstruction* round) {
    return UnsupportedTypeError(round);
  }

  Status HandleRound(HloInstruction* round) override {
    return HandleRound<ReturnT>(round);
  }

  template <
      typename NativeT,
      typename std::enable_if<!is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleCeil(HloInstruction* ceil) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[ceil],
                        ElementWiseUnaryOp(ceil, [](ElementwiseT elem_operand) {
                          return std::ceil(elem_operand);
                        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleCeil(HloInstruction* ceil) {
    return UnsupportedTypeError(ceil);
  }

  Status HandleCeil(HloInstruction* ceil) override {
    return HandleCeil<ReturnT>(ceil);
  }

  Status HandleConvert(HloInstruction* convert) override {
    const HloInstruction* operand = convert->operand(0);
    TF_RET_CHECK(ShapeUtil::SameDimensions(operand->shape(), convert->shape()));
    TF_ASSIGN_OR_RETURN(Literal result,
                        parent_->GetEvaluatedLiteralFor(operand).Convert(
                            convert->shape().element_type()));
    parent_->evaluated_[convert] = std::move(result);
    return Status::OK();
  }

  Status HandleBitcastConvert(HloInstruction* convert) override {
    const HloInstruction* operand = convert->operand(0);
    TF_RET_CHECK(ShapeUtil::SameDimensions(operand->shape(), convert->shape()));
    TF_ASSIGN_OR_RETURN(Literal result,
                        parent_->GetEvaluatedLiteralFor(operand).BitcastConvert(
                            convert->shape().element_type()));

    parent_->evaluated_[convert] = std::move(result);
    return Status::OK();
  }

  Status HandleExp(HloInstruction* exp) override {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[exp],
                        ElementWiseUnaryOp(exp, [](ElementwiseT elem_operand) {
                          return std::exp(elem_operand);
                        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<!is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleExpm1(HloInstruction* expm1) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[expm1],
        ElementWiseUnaryOp(expm1, [](ElementwiseT elem_operand) {
          return std::expm1(elem_operand);
        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleExpm1(HloInstruction* expm1) {
    return UnsupportedTypeError(expm1);
  }

  Status HandleExpm1(HloInstruction* floor) override {
    return HandleExpm1<ReturnT>(floor);
  }

  template <
      typename NativeT,
      typename std::enable_if<!is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleFloor(HloInstruction* floor) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[floor],
        ElementWiseUnaryOp(floor, [](ElementwiseT elem_operand) {
          return std::floor(elem_operand);
        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleFloor(HloInstruction* floor) {
    return UnsupportedTypeError(floor);
  }

  Status HandleFloor(HloInstruction* floor) override {
    return HandleFloor<ReturnT>(floor);
  }

  Status HandleLog(HloInstruction* log) override {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[log],
                        ElementWiseUnaryOp(log, [](ElementwiseT elem_operand) {
                          return std::log(elem_operand);
                        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<!is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleLog1p(HloInstruction* expm1) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[expm1],
        ElementWiseUnaryOp(expm1, [](ElementwiseT elem_operand) {
          return std::log1p(elem_operand);
        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleLog1p(HloInstruction* log1p) {
    return UnsupportedTypeError(log1p);
  }

  Status HandleLog1p(HloInstruction* log1p) override {
    return HandleLog1p<ReturnT>(log1p);
  }

  template <typename NativeT,
            typename std::enable_if<
                std::is_integral<NativeT>::value &&
                !std::is_same<NativeT, bool>::value>::type* = nullptr>
  Status HandleNot(HloInstruction* not_) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[not_],
                        ElementWiseUnaryOp(not_, [](ElementwiseT elem_operand) {
                          return ~elem_operand;
                        }));
    return Status::OK();
  }

  template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleNot(HloInstruction* not_) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[not_],
                        ElementWiseUnaryOp(not_, [](ElementwiseT elem_operand) {
                          return !elem_operand;
                        }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<std::is_same<NativeT, bool>::value>::type* =
                nullptr>
  Status HandleNot(HloInstruction* not_) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[not_],
                        ElementWiseUnaryOp(not_, [](ElementwiseT elem_operand) {
                          return !elem_operand;
                        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleNot(HloInstruction* not_) {
    return UnsupportedTypeError(not_);
  }

  Status HandleNot(HloInstruction* not_) override {
    return HandleNot<ElementwiseT>(not_);
  }

  template <typename NativeT,
            typename std::enable_if<
                std::is_signed<NativeT>::value &&
                !std::is_floating_point<NativeT>::value>::type* = nullptr>
  Status HandleNegate(HloInstruction* negate) {
    using type = typename std::make_unsigned<NativeT>::type;
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[negate],
        ElementWiseUnaryOp(negate, [](ElementwiseT elem_operand) {
          return NativeT(-type(elem_operand));
        }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<
                !std::is_signed<NativeT>::value ||
                std::is_floating_point<NativeT>::value>::type* = nullptr>
  Status HandleNegate(HloInstruction* negate) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[negate],
        ElementWiseUnaryOp(
            negate, [](ElementwiseT elem_operand) { return -elem_operand; }));
    return Status::OK();
  }

  Status HandleNegate(HloInstruction* negate) override {
    return HandleNegate<ReturnT>(negate);
  }

  template <
      typename NativeT,
      typename std::enable_if<!is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleSign(HloInstruction* sign) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[sign],
                        ElementWiseUnaryOp(sign, [](ElementwiseT elem_operand) {
                          return (ElementwiseT(0) < elem_operand) -
                                 (elem_operand < ElementwiseT(0));
                        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleSign(HloInstruction* sign) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[sign],
                        ElementWiseUnaryOp(sign, [](ElementwiseT elem_operand) {
                          auto abs_val = std::abs(elem_operand);
                          return 0 == abs_val ? ElementwiseT(0)
                                              : elem_operand / abs_val;
                        }));
    return Status::OK();
  }

  Status HandleSign(HloInstruction* sign) override {
    return HandleSign<ReturnT>(sign);
  }

  template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleAtan2(HloInstruction* atan2) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[atan2],
                        ElementWiseBinaryOp(atan2, [](ElementwiseT lhs_elem,
                                                      ElementwiseT rhs_elem) {
                          return std::atan2(lhs_elem, rhs_elem);
                        }));
    return Status::OK();
  }

  template <typename NativeT, typename std::enable_if<!std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleAtan2(HloInstruction* atan2) {
    return UnsupportedTypeError(atan2);
  }

  Status HandleAtan2(HloInstruction* atan2) override {
    return HandleAtan2<ElementwiseT>(atan2);
  }

  Status HandleTanh(HloInstruction* tanh) override {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[tanh],
                        ElementWiseUnaryOp(tanh, [](ElementwiseT elem_operand) {
                          return std::tanh(elem_operand);
                        }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<
                std::is_signed<NativeT>::value &&
                !std::is_floating_point<NativeT>::value>::type* = nullptr>
  Status HandleMultiply(HloInstruction* multiply) {
    using type = typename std::make_unsigned<NativeT>::type;
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[multiply],
        ElementWiseBinaryOp(multiply,
                            [](ElementwiseT lhs_elem, ElementwiseT rhs_elem) {
                              return NativeT(type(lhs_elem) * type(rhs_elem));
                            }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<std::is_unsigned<NativeT>::value ||
                              std::is_floating_point<NativeT>::value ||
                              is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleMultiply(HloInstruction* multiply) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[multiply],
        ElementWiseBinaryOp(multiply,
                            [](ElementwiseT lhs_elem, ElementwiseT rhs_elem) {
                              return lhs_elem * rhs_elem;
                            }));
    return Status::OK();
  }

  Status HandleMultiply(HloInstruction* multiply) override {
    return HandleMultiply<ElementwiseT>(multiply);
  }

  Status HandleSubtract(HloInstruction* subtract) override {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[subtract],
        ElementWiseBinaryOp(subtract,
                            [](ElementwiseT lhs_elem, ElementwiseT rhs_elem) {
                              return lhs_elem - rhs_elem;
                            }));
    return Status::OK();
  }

  Status HandleAdd(HloInstruction* add) override {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[add],
                        ElementWiseBinaryOp(add, [](ElementwiseT lhs_elem,
                                                    ElementwiseT rhs_elem) {
                          return lhs_elem + rhs_elem;
                        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<std::is_floating_point<NativeT>::value ||
                              is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleDivide(HloInstruction* divide) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[divide],
                        ElementWiseBinaryOp(divide, [](ElementwiseT lhs_elem,
                                                       ElementwiseT rhs_elem) {
                          return lhs_elem / rhs_elem;
                        }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<std::is_signed<NativeT>::value &&
                                    std::is_integral<NativeT>::value>::type* =
                nullptr>
  Status HandleDivide(HloInstruction* divide) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[divide],
        ElementWiseBinaryOp(
            divide,
            [](ElementwiseT lhs_elem, ElementwiseT rhs_elem) -> ElementwiseT {
              if (rhs_elem == 0) {
                return static_cast<ElementwiseT>(-1);
              }
              if (rhs_elem == -1 &&
                  lhs_elem == std::numeric_limits<ElementwiseT>::min()) {
                return lhs_elem;
              }
              return lhs_elem / rhs_elem;
            }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<std::is_unsigned<NativeT>::value>::type* =
                nullptr>
  Status HandleDivide(HloInstruction* divide) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[divide],
                        ElementWiseBinaryOp(divide, [](ElementwiseT lhs_elem,
                                                       ElementwiseT rhs_elem) {
                          return rhs_elem == 0
                                     ? std::numeric_limits<ElementwiseT>::max()
                                     : (lhs_elem / rhs_elem);
                        }));
    return Status::OK();
  }

  Status HandleDivide(HloInstruction* divide) override {
    return HandleDivide<ElementwiseT>(divide);
  }

  template <typename NativeT,
            typename std::enable_if<std::is_integral<NativeT>::value>::type* =
                nullptr>
  Status HandleMaximum(HloInstruction* maximum) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[maximum],
        ElementWiseBinaryOp(maximum, [](ElementwiseT lhs, ElementwiseT rhs) {
          return std::max(lhs, rhs);
        }));
    return Status::OK();
  }

  template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleMaximum(HloInstruction* maximum) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[maximum],
        ElementWiseBinaryOp(maximum, [](ElementwiseT lhs, ElementwiseT rhs) {
          return ((lhs >= rhs) || std::isnan(lhs)) ? lhs : rhs;
        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleMaximum(HloInstruction* maximum) {
    return UnsupportedTypeError(maximum);
  }

  Status HandleMaximum(HloInstruction* maximum) override {
    return HandleMaximum<ElementwiseT>(maximum);
  }

  template <typename NativeT,
            typename std::enable_if<std::is_integral<NativeT>::value>::type* =
                nullptr>
  Status HandleMinimum(HloInstruction* minimum) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[minimum],
                        ElementWiseBinaryOp(minimum, [](ElementwiseT lhs_el,
                                                        ElementwiseT rhs_el) {
                          return std::min(lhs_el, rhs_el);
                        }));
    return Status::OK();
  }

  template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleMinimum(HloInstruction* minimum) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[minimum],
        ElementWiseBinaryOp(minimum, [](ElementwiseT lhs_el,
                                        ElementwiseT rhs_el) {
          return ((lhs_el <= rhs_el) || std::isnan(lhs_el)) ? lhs_el : rhs_el;
        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleMinimum(HloInstruction* minimum) {
    return UnsupportedTypeError(minimum);
  }

  Status HandleMinimum(HloInstruction* minimum) override {
    return HandleMinimum<ElementwiseT>(minimum);
  }

  Status HandlePower(HloInstruction* power) override {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[power],
                        ElementWiseBinaryOp(power, [](ElementwiseT lhs_el,
                                                      ElementwiseT rhs_el) {
                          return std::pow(lhs_el, rhs_el);
                        }));
    return Status::OK();
  }

  template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleRemainder(HloInstruction* remainder) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[remainder],
                        ElementWiseBinaryOp(remainder, [](ElementwiseT lhs_el,
                                                          ElementwiseT rhs_el) {
                          return std::fmod(lhs_el, rhs_el);
                        }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<std::is_unsigned<NativeT>::value>::type* =
                nullptr>
  Status HandleRemainder(HloInstruction* remainder) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[remainder],
                        ElementWiseBinaryOp(remainder, [](ElementwiseT lhs_el,
                                                          ElementwiseT rhs_el) {
                          return rhs_el == 0 ? lhs_el : (lhs_el % rhs_el);
                        }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<std::is_signed<NativeT>::value &&
                                    std::is_integral<NativeT>::value>::type* =
                nullptr>
  Status HandleRemainder(HloInstruction* remainder) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[remainder],
        ElementWiseBinaryOp(
            remainder,
            [](ElementwiseT lhs_el, ElementwiseT rhs_el) -> ElementwiseT {
              if (rhs_el == 0) {
                return lhs_el;
              }
              if (rhs_el == -1 &&
                  lhs_el == std::numeric_limits<ElementwiseT>::min()) {
                return 0;
              }
              return lhs_el % rhs_el;
            }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleRemainder(HloInstruction* remainder) {
    return UnsupportedTypeError(remainder);
  }

  Status HandleRemainder(HloInstruction* remainder) override {
    return HandleRemainder<ElementwiseT>(remainder);
  }

  template <typename NativeT,
            typename std::enable_if<std::is_integral<NativeT>::value>::type* =
                nullptr>
  Status HandleAnd(HloInstruction* and_) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[and_],
        ElementWiseBinaryOp(and_, [](ElementwiseT lhs_el, ElementwiseT rhs_el) {
          return lhs_el & rhs_el;
        }));
    return Status::OK();
  }

  template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleAnd(HloInstruction* and_) {
    return UnsupportedTypeError(and_);
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleAnd(HloInstruction* and_) {
    return UnsupportedTypeError(and_);
  }

  Status HandleAnd(HloInstruction* and_) override {
    return HandleAnd<ElementwiseT>(and_);
  }

  template <typename NativeT,
            typename std::enable_if<std::is_integral<NativeT>::value>::type* =
                nullptr>
  Status HandleOr(HloInstruction* or_) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[or_],
        ElementWiseBinaryOp(or_, [](ElementwiseT lhs_el, ElementwiseT rhs_el) {
          return lhs_el | rhs_el;
        }));
    return Status::OK();
  }

  template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleOr(HloInstruction* or_) {
    return UnsupportedTypeError(or_);
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleOr(HloInstruction* or_) {
    return InvalidArgument("Unsupported type for Or");
  }

  Status HandleOr(HloInstruction* or_) override {
    return HandleOr<ElementwiseT>(or_);
  }

  template <typename NativeT,
            typename std::enable_if<std::is_integral<NativeT>::value>::type* =
                nullptr>
  Status HandleXor(HloInstruction* xor_) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[xor_],
        ElementWiseBinaryOp(xor_, [](ElementwiseT lhs_el, ElementwiseT rhs_el) {
          return lhs_el ^ rhs_el;
        }));
    return Status::OK();
  }

  template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleXor(HloInstruction* xor_) {
    return UnsupportedTypeError(xor_);
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleXor(HloInstruction* xor_) {
    return UnsupportedTypeError(xor_);
  }

  Status HandleXor(HloInstruction* xor_) override {
    return HandleXor<ElementwiseT>(xor_);
  }

  template <typename NativeT,
            typename std::enable_if<
                std::is_integral<NativeT>::value &&
                !std::is_same<NativeT, bool>::value>::type* = nullptr>
  Status HandleShiftLeft(HloInstruction* shl) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[shl],
        ElementWiseBinaryOp(shl, [](NativeT lhs_elem, NativeT rhs_elem) {
          return IsShiftOutOfBounds<NativeT>(rhs_elem) ? 0
                                                       : (lhs_elem << rhs_elem);
        }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<!std::is_integral<NativeT>::value ||
                                    std::is_same<NativeT, bool>::value>::type* =
                nullptr>
  Status HandleShiftLeft(HloInstruction* shift) {
    return UnsupportedTypeError(shift);
  }

  Status HandleShiftLeft(HloInstruction* shl) override {
    return HandleShiftLeft<ElementwiseT>(shl);
  }
  template <typename NativeT,
            typename std::enable_if<
                std::is_integral<NativeT>::value &&
                !std::is_same<NativeT, bool>::value>::type* = nullptr>
  Status HandleShiftRightArithmetic(HloInstruction* shr) {
    typedef typename std::make_signed<NativeT>::type SignedT;
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[shr],
        ElementWiseBinaryOp(shr, [](NativeT lhs_elem, NativeT rhs_elem) {
          SignedT lhs_signed = static_cast<SignedT>(lhs_elem);
          if (IsShiftOutOfBounds<NativeT>(rhs_elem)) {
            return lhs_signed < 0 ? static_cast<SignedT>(-1) : 0;
          } else {
            return lhs_signed >> rhs_elem;
          }
        }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<!std::is_integral<NativeT>::value ||
                                    std::is_same<NativeT, bool>::value>::type* =
                nullptr>
  Status HandleShiftRightArithmetic(HloInstruction* shift) {
    return UnsupportedTypeError(shift);
  }

  Status HandleShiftRightArithmetic(HloInstruction* shra) override {
    return HandleShiftRightArithmetic<ElementwiseT>(shra);
  }

  template <typename NativeT,
            typename std::enable_if<
                std::is_integral<NativeT>::value &&
                !std::is_same<NativeT, bool>::value>::type* = nullptr>
  Status HandleShiftRightLogical(HloInstruction* shr) {
    typedef typename std::make_unsigned<NativeT>::type UnsignedT;
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[shr],
        ElementWiseBinaryOp(shr, [](NativeT lhs_elem, NativeT rhs_elem) {
          // If shift amount is greater than the number of bits, then return 0.
          if (IsShiftOutOfBounds<NativeT>(rhs_elem)) {
            return static_cast<NativeT>(0);
          }
          return static_cast<NativeT>(static_cast<UnsignedT>(lhs_elem) >>
                                      rhs_elem);
        }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<!std::is_integral<NativeT>::value ||
                                    std::is_same<NativeT, bool>::value>::type* =
                nullptr>
  Status HandleShiftRightLogical(HloInstruction* shift) {
    return UnsupportedTypeError(shift);
  }

  Status HandleShiftRightLogical(HloInstruction* shrl) override {
    return HandleShiftRightLogical<ElementwiseT>(shrl);
  }

  template <
      typename NativeT,
      typename std::enable_if<!is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleClamp(HloInstruction* clamp) {
    std::function<ElementwiseT(ElementwiseT, ElementwiseT, ElementwiseT)>
        clamp_op = [](ElementwiseT low, ElementwiseT value, ElementwiseT high) {
          return std::fmin(high, std::fmax(value, low));
        };
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[clamp],
        ElementwiseTernaryOp(clamp,
                             std::move(ConvertTernaryFunction(clamp_op))));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleClamp(HloInstruction* clamp) {
    return UnsupportedTypeError(clamp);
  }

  Status HandleClamp(HloInstruction* clamp) override {
    return HandleClamp<ElementwiseT>(clamp);
  }

  Status HandleSelect(HloInstruction* select) override {
    CHECK(!ShapeUtil::IsScalar(select->operand(0)->shape()));
    CHECK(ShapeUtil::IsArray(select->shape()));
    std::function<ReturnT(bool, ReturnT, ReturnT)> select_op =
        [](bool pred, ReturnT on_true, ReturnT on_false) {
          if (pred) {
            return on_true;
          }
          return on_false;
        };
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[select],
                        ElementwiseTernaryOp(select, std::move(select_op)));
    return Status::OK();
  }

  Status HandleReverse(HloInstruction* reverse) override {
    const auto result_shape = reverse->shape();
    const auto reverse_dimensions = reverse->dimensions();

    auto operand = reverse->operand(0);
    TF_ASSIGN_OR_RETURN(auto inferred_return_shape,
                        ShapeInference::InferReverseShape(operand->shape(),
                                                          reverse_dimensions));

    TF_RET_CHECK(ShapeUtil::Compatible(result_shape, inferred_return_shape))
        << "return shape set to: " << ShapeUtil::HumanString(result_shape)
        << " but is inferred to be: "
        << ShapeUtil::HumanString(inferred_return_shape);

    const Literal& operand_literal = parent_->GetEvaluatedLiteralFor(operand);
    Literal result(result_shape);

    TF_RETURN_IF_ERROR(
        result.Populate<ReturnT>([&](absl::Span<const int64> out_index) {
          std::vector<int64> from_index(out_index.begin(), out_index.end());
          for (const int64 dim : reverse_dimensions) {
            from_index[dim] = result_shape.dimensions(dim) - 1 - out_index[dim];
          }
          return operand_literal.Get<ReturnT>(from_index);
        }));

    parent_->evaluated_[reverse] = std::move(result);
    return Status::OK();
  }

  Status HandleConvolution(HloInstruction* conv) override {
    auto lhs = conv->operand(0);
    auto rhs = conv->operand(1);
    const auto& window = conv->window();
    const Shape& result_shape = conv->shape();
    const Shape& lhs_shape = lhs->shape();
    const Shape& rhs_shape = rhs->shape();

    TF_CHECK_OK(ShapeUtil::ValidateShape(lhs_shape));
    TF_CHECK_OK(ShapeUtil::ValidateShape(rhs_shape));
    CHECK(ShapeUtil::IsArray(lhs_shape));
    CHECK(ShapeUtil::IsArray(rhs_shape));
    CHECK(ShapeUtil::SameElementType(lhs_shape, rhs_shape));
    CHECK(ShapeUtil::SameElementType(lhs_shape, result_shape));

    const auto& dnums = conv->convolution_dimension_numbers();
    const int64 num_spatial_dims = dnums.output_spatial_dimensions_size();
    CHECK_EQ(num_spatial_dims, dnums.input_spatial_dimensions_size());
    CHECK_EQ(num_spatial_dims, dnums.kernel_spatial_dimensions_size());
    CHECK_GE(num_spatial_dims, 0);
    CHECK_EQ(window.dimensions_size(), num_spatial_dims);

    const auto lhs_rank = ShapeUtil::Rank(lhs_shape);
    const auto rhs_rank = ShapeUtil::Rank(rhs_shape);

    CHECK_EQ(num_spatial_dims + 2, lhs_rank);
    CHECK_EQ(num_spatial_dims + 2, rhs_rank);

    TF_ASSIGN_OR_RETURN(
        auto inferred_return_shape,
        ShapeInference::InferConvolveShape(
            lhs_shape, rhs_shape, conv->feature_group_count(), window, dnums));
    CHECK(ShapeUtil::Compatible(result_shape, inferred_return_shape))
        << "return shape set to: " << ShapeUtil::HumanString(result_shape)
        << " but is inferred to be: "
        << ShapeUtil::HumanString(inferred_return_shape);

    const Literal& lhs_literal = parent_->GetEvaluatedLiteralFor(lhs);
    const Literal& rhs_literal = parent_->GetEvaluatedLiteralFor(rhs);

    std::vector<int64> window_dimension_sizes;
    for (auto i : dnums.kernel_spatial_dimensions()) {
      window_dimension_sizes.push_back(ShapeUtil::GetDimension(rhs_shape, i));
    }

    const Shape& window_shape =
        ShapeUtil::MakeShape(rhs_shape.element_type(), window_dimension_sizes);

    DimensionVector lhs_dim_multipliers = MakeDimMultipliers(lhs_shape);
    DimensionVector rhs_dim_multipliers = MakeDimMultipliers(rhs_shape);

    auto lhs_literal_data = lhs_literal.data<ReturnT>();
    auto rhs_literal_data = rhs_literal.data<ReturnT>();

    int64 feature_group_count = conv->feature_group_count();

    auto func = [&window_shape, &dnums, &lhs_shape, &rhs_shape, &window,
                 &lhs_dim_multipliers, &rhs_dim_multipliers, lhs_literal_data,
                 rhs_literal_data,
                 feature_group_count](const absl::Span<const int64> out_index) {
      // Dimension number applicable for input (lhs).
      const int64 input_batch_dim = dnums.input_batch_dimension();
      const int64 input_z_dim = dnums.input_feature_dimension();
      // Dimension number applicable for kernel (rhs).
      const int64 kernel_input_z_dim = dnums.kernel_input_feature_dimension();
      const int64 kernel_output_z_dim = dnums.kernel_output_feature_dimension();
      // Dimension number applicable for output.
      const int64 output_batch_dim = dnums.output_batch_dimension();
      const int64 output_z_dim = dnums.output_feature_dimension();

      const int64 input_z_size =
          ShapeUtil::GetDimension(lhs_shape, input_z_dim);
      // The size of an input feature group.
      const int64 input_feature_group_size = input_z_size / feature_group_count;

      const int64 output_z_size =
          ShapeUtil::GetDimension(rhs_shape, kernel_output_z_dim);
      // The output feature dimension is a concatenation of convolution results
      // from the different groups.
      const int64 output_feature_group_size =
          output_z_size / feature_group_count;

      // Calculate the group index to which the current output index
      // belongs.
      const int64 feature_group_index =
          out_index[output_z_dim] / output_feature_group_size;

      ElementwiseT result_val = static_cast<ElementwiseT>(0);
      DimensionVector rhs_spatial_index(dnums.kernel_spatial_dimensions_size(),
                                        0);

      // Convolve input feature with kernel.
      do {
        // Find corresponding spatial dimension index for input (lhs).
        int64 lhs_linear_spatial_index = 0;
        int64 rhs_linear_spatial_index = 0;
        for (int64 ki = 0; ki < rhs_spatial_index.size(); ++ki) {
          // Spatial dimension number for input (lhs) and output.
          const int64 input_spatial_dim = dnums.input_spatial_dimensions(ki);
          const int64 output_spatial_dim = dnums.output_spatial_dimensions(ki);

          // Calculate lhs (input) index without taking base dilation into
          // account.
          const auto& window_dim = window.dimensions(ki);
          const int64 undilated_index =
              out_index[output_spatial_dim] * window_dim.stride() -
              window_dim.padding_low() +
              rhs_spatial_index[ki] * window_dim.window_dilation();
          // Skip if the lhs (input) index is to be dilated.  As an
          // optimization, skip this mod if there's no dilation.
          if (window_dim.base_dilation() > 1 &&
              undilated_index % window_dim.base_dilation() != 0) {
            goto cnt;
          }

          // Calculate the actual lhs (input) index after dilation.  As an
          // optimization, skip this integer divide if there's no dilation.
          int64 lhs_spatial_index;
          if (window_dim.base_dilation() > 1) {
            lhs_spatial_index = undilated_index / window_dim.base_dilation();
          } else {
            lhs_spatial_index = undilated_index;
          }

          // Skip if input index is not in bounds.
          if (!(lhs_spatial_index >= 0 &&
                lhs_spatial_index < lhs_shape.dimensions(input_spatial_dim))) {
            goto cnt;
          }

          lhs_linear_spatial_index +=
              lhs_spatial_index * lhs_dim_multipliers[input_spatial_dim];
          rhs_linear_spatial_index +=
              (window_dim.window_reversal()
                   ? ((window_dim.size() - 1) - rhs_spatial_index[ki])
                   : rhs_spatial_index[ki]) *
              rhs_dim_multipliers[dnums.kernel_spatial_dimensions(ki)];
        }

        for (int64 rhs_iz = 0; rhs_iz < input_feature_group_size; ++rhs_iz) {
          const int64 iz =
              feature_group_index * input_feature_group_size + rhs_iz;

          int64 lhs_linear_index = lhs_linear_spatial_index;
          lhs_linear_index += out_index[output_batch_dim] *
                              lhs_dim_multipliers[input_batch_dim];
          lhs_linear_index += iz * lhs_dim_multipliers[input_z_dim];

          int64 rhs_linear_index = rhs_linear_spatial_index;
          rhs_linear_index += out_index[output_z_dim] *
                              rhs_dim_multipliers[kernel_output_z_dim];
          rhs_linear_index += rhs_iz * rhs_dim_multipliers[kernel_input_z_dim];

          result_val +=
              static_cast<ElementwiseT>(lhs_literal_data[lhs_linear_index]) *
              static_cast<ElementwiseT>(rhs_literal_data[rhs_linear_index]);
        }
      cnt : {}
      } while (IndexUtil::BumpIndices(window_shape,
                                      absl::MakeSpan(rhs_spatial_index)));

      return static_cast<ReturnT>(result_val);
    };

    Literal result(result_shape);
    TF_RETURN_IF_ERROR(result.PopulateParallel<ReturnT>(func));

    parent_->evaluated_[conv] = std::move(result);
    return Status::OK();
  }

  Status HandleDot(HloInstruction* dot) override {
    if (parent_->use_fast_path_) {
      return HandleDot<ReturnT>(dot);
    }
    return HandleDotSlowPath(dot);
  }

  template <typename NativeT, typename std::enable_if<std::is_same<
                                  NativeT, float>::value>::type* = nullptr>
  Status HandleDot(HloInstruction* dot) {
    const HloInstruction* lhs = dot->operand(0);
    const HloInstruction* rhs = dot->operand(1);
    CHECK(ShapeUtil::IsArray(dot->shape()));
    CHECK(ShapeUtil::IsArray(lhs->shape()));
    CHECK(ShapeUtil::IsArray(rhs->shape()));

    const auto& dnums = dot->dot_dimension_numbers();

    const int64 lhs_rank = ShapeUtil::Rank(lhs->shape());
    const int64 rhs_rank = ShapeUtil::Rank(rhs->shape());

    CHECK(ShapeUtil::SameElementType(lhs->shape(), rhs->shape()));
    CHECK(ShapeUtil::SameElementType(lhs->shape(), dot->shape()));

    // There must be 1 and only 1 Contracting dimension for lhs and rhs.
    CHECK_EQ(dnums.lhs_contracting_dimensions_size(), 1);
    CHECK_EQ(dnums.rhs_contracting_dimensions_size(), 1);
    const int64 lhs_contracting_dimension = dnums.lhs_contracting_dimensions(0);
    const int64 rhs_contracting_dimension = dnums.rhs_contracting_dimensions(0);
    // Contracted dimension sizes must be the same.
    CHECK_EQ(lhs->shape().dimensions(lhs_contracting_dimension),
             rhs->shape().dimensions(rhs_contracting_dimension))
        << "lhs contracted dimension: "
        << lhs->shape().dimensions(lhs_contracting_dimension)
        << " rhs contracted dimension: "
        << rhs->shape().dimensions(rhs_contracting_dimension);

    // The fast path is for a simple rank 2 dot with default layout operands.
    if (lhs_rank == 2 && rhs_rank == 2 && lhs_contracting_dimension == 1 &&
        rhs_contracting_dimension == 0 &&
        LayoutUtil::Equal(lhs->shape().layout(),
                          LayoutUtil::GetDefaultLayoutForR2()) &&
        LayoutUtil::Equal(rhs->shape().layout(),
                          LayoutUtil::GetDefaultLayoutForR2()) &&
        LayoutUtil::Equal(dot->shape().layout(),
                          LayoutUtil::GetDefaultLayoutForR2())) {
      const Literal& lhs_literal = parent_->GetEvaluatedLiteralFor(lhs);
      const Literal& rhs_literal = parent_->GetEvaluatedLiteralFor(rhs);
      const int64 contracted_dimension_size =
          lhs->shape().dimensions(lhs_contracting_dimension);
      Array2D<NativeT> lhs_array(lhs->shape().dimensions(0),
                                 contracted_dimension_size);
      lhs_array.SetValues(lhs_literal.data<NativeT>());
      Array2D<NativeT> rhs_array(contracted_dimension_size,
                                 rhs->shape().dimensions(1));
      rhs_array.SetValues(rhs_literal.data<NativeT>());
      std::unique_ptr<Array2D<NativeT>> result_array =
          HloEvaluator::MatmulArray2D(lhs_array, rhs_array);
      Literal result(dot->shape());
      result.PopulateR2FromArray2D(*result_array);
      parent_->evaluated_[dot] = std::move(result);
      return Status::OK();
    }
    return HandleDotSlowPath(dot);
  }

  template <typename NativeT, typename std::enable_if<!std::is_same<
                                  NativeT, float>::value>::type* = nullptr>
  Status HandleDot(HloInstruction* dot) {
    return HandleDotSlowPath(dot);
  }

  Status HandleDotSlowPath(HloInstruction* dot) {
    auto lhs = dot->operand(0);
    auto rhs = dot->operand(1);
    CHECK(ShapeUtil::IsArray(dot->shape()));
    CHECK(ShapeUtil::IsArray(lhs->shape()));
    CHECK(ShapeUtil::IsArray(rhs->shape()));

    const auto& dnums = dot->dot_dimension_numbers();

    const auto lhs_rank = ShapeUtil::Rank(lhs->shape());
    const auto rhs_rank = ShapeUtil::Rank(rhs->shape());

    CHECK(ShapeUtil::SameElementType(lhs->shape(), rhs->shape()));
    CHECK(ShapeUtil::SameElementType(lhs->shape(), dot->shape()));

    // There must be 1 and only 1 Contracting dimension for lhs and rhs.
    CHECK_EQ(dnums.lhs_contracting_dimensions_size(), 1);
    CHECK_EQ(dnums.rhs_contracting_dimensions_size(), 1);
    const int64 lhs_contracting_dimension = dnums.lhs_contracting_dimensions(0);
    const int64 rhs_contracting_dimension = dnums.rhs_contracting_dimensions(0);
    // Contracted dimension sizes must be the same.
    CHECK_EQ(lhs->shape().dimensions(lhs_contracting_dimension),
             rhs->shape().dimensions(rhs_contracting_dimension))
        << "lhs contracted dimension: "
        << lhs->shape().dimensions(lhs_contracting_dimension)
        << " rhs contracted dimension: "
        << rhs->shape().dimensions(rhs_contracting_dimension);
    const int64 contracted_dimension_size =
        lhs->shape().dimensions(lhs_contracting_dimension);

    const Literal& lhs_literal = parent_->GetEvaluatedLiteralFor(lhs);
    const Literal& rhs_literal = parent_->GetEvaluatedLiteralFor(rhs);

    CHECK_EQ(dnums.lhs_batch_dimensions_size(),
             dnums.rhs_batch_dimensions_size());

    DimensionVector lhs_index(lhs_rank);
    DimensionVector rhs_index(rhs_rank);

    // result_index_locations[i] contains one or two pointers to the locations
    // in lhs_index or rhs_index where the i'th result index should go.
    absl::InlinedVector<std::pair<int64*, int64*>, kInlineRank>
        result_index_locations;
    result_index_locations.reserve(lhs_rank + rhs_rank - 2);

    // The first components in the output shape are the LHS and RHS batch
    // dimensions:
    for (int64 i = 0; i < dnums.lhs_batch_dimensions_size(); i++) {
      result_index_locations.push_back(
          {&lhs_index[dnums.lhs_batch_dimensions(i)],
           &rhs_index[dnums.rhs_batch_dimensions(i)]});
    }

    // Then we have the LHS and RHS non-contracting dimensions, if any:
    for (int64 i = 0; i < lhs_rank; i++) {
      if (i != lhs_contracting_dimension &&
          !absl::c_linear_search(dnums.lhs_batch_dimensions(), i)) {
        result_index_locations.push_back({&lhs_index[i], nullptr});
      }
    }
    for (int64 i = 0; i < rhs_rank; i++) {
      if (i != rhs_contracting_dimension &&
          !absl::c_linear_search(dnums.rhs_batch_dimensions(), i)) {
        result_index_locations.push_back({&rhs_index[i], nullptr});
      }
    }

    Literal result(dot->shape());
    TF_RETURN_IF_ERROR(
        result.Populate<ReturnT>([&](absl::Span<const int64> result_index) {
          ElementwiseT result_val = static_cast<ElementwiseT>(0);

          for (int64 i = 0; i < result_index.size(); i++) {
            *result_index_locations[i].first = result_index[i];
            if (result_index_locations[i].second) {
              *result_index_locations[i].second = result_index[i];
            }
          }

          // Accumulates resulting product along the contracted dimension.
          for (int64 i = 0; i < contracted_dimension_size; ++i) {
            lhs_index[lhs_contracting_dimension] = i;
            rhs_index[rhs_contracting_dimension] = i;

            result_val +=
                static_cast<ElementwiseT>(lhs_literal.Get<ReturnT>(lhs_index)) *
                static_cast<ElementwiseT>(rhs_literal.Get<ReturnT>(rhs_index));
          }

          return static_cast<ReturnT>(result_val);
        }));

    parent_->evaluated_[dot] = std::move(result);
    return Status::OK();
  }

  Status HandlePad(HloInstruction* pad) override {
    CHECK(ShapeUtil::IsArray(pad->operand(0)->shape()));
    // Padding value must be scalar.
    CHECK(ShapeUtil::IsScalar(pad->operand(1)->shape()));
    CHECK_EQ(ShapeUtil::Rank(pad->operand(0)->shape()),
             pad->padding_config().dimensions_size());

    TF_ASSIGN_OR_RETURN(auto inferred_return_shape,
                        ShapeInference::InferPadShape(
                            /*operand_shape=*/pad->operand(0)->shape(),
                            /*padding_value_shape=*/pad->operand(1)->shape(),
                            /*padding_config=*/pad->padding_config()));
    CHECK(ShapeUtil::Compatible(pad->shape(), inferred_return_shape))
        << "return shape is set to: " << ShapeUtil::HumanString(pad->shape())
        << " but is inferred to be: "
        << ShapeUtil::HumanString(inferred_return_shape);

    // Create new HLO of padded shape with padding value.
    ReturnT scalar =
        parent_->GetEvaluatedLiteralFor(pad->operand(1)).Get<ReturnT>({});
    Literal result(pad->shape());
    TF_RETURN_IF_ERROR(result.Populate<ReturnT>(
        [&scalar](absl::Span<const int64> multi_index) { return scalar; }));

    const Literal& evaluated_operand =
        parent_->GetEvaluatedLiteralFor(pad->operand(0));

    std::vector<int64> input_index(ShapeUtil::Rank(evaluated_operand.shape()),
                                   0);
    std::vector<int64> target_index(ShapeUtil::Rank(result.shape()), 0);

    // Loop through each element of the operand, assign them to the
    // corresponding index of the resulting padded literal.
    const PaddingConfig& pad_config = pad->padding_config();

    auto func = [&](absl::Span<const int64> input_index) {
      for (auto i = 0; i < input_index.size(); ++i) {
        // Interior padding occurs logically before edge padding, so in the case
        // of negative edge padding elements are removed from the
        // interior-padded operand.
        target_index[i] =
            pad_config.dimensions(i).edge_padding_low() +
            input_index[i] * (pad_config.dimensions(i).interior_padding() + 1);

        // Account for negative low and high padding: skip assignment if the
        // any target index is out of range.
        if (!(target_index[i] >= 0 &&
              target_index[i] < pad->shape().dimensions(i))) {
          return true;
        }
      }
      result.Set<ReturnT>(target_index,
                          evaluated_operand.Get<ReturnT>(input_index));
      return true;
    };

    std::vector<int64> zero_base(evaluated_operand.shape().dimensions_size(),
                                 0);
    std::vector<int64> step(evaluated_operand.shape().dimensions_size(), 1);

    ShapeUtil::ForEachIndex(
        evaluated_operand.shape(), zero_base,
        AsInt64Slice(evaluated_operand.shape().dimensions()), step, func);

    parent_->evaluated_[pad] = std::move(result);
    return Status::OK();
  }

  Status HandleDynamicSlice(HloInstruction* dynamic_slice) override {
    auto operand = dynamic_slice->operand(0);
    auto start_indices = dynamic_slice->operand(1);
    auto result_shape = dynamic_slice->shape();
    TF_ASSIGN_OR_RETURN(auto inferred_return_shape,
                        ShapeInference::InferDynamicSliceShape(
                            operand->shape(), start_indices->shape(),
                            dynamic_slice->dynamic_slice_sizes()));
    TF_RET_CHECK(ShapeUtil::Compatible(result_shape, inferred_return_shape))
        << "return shape is set to: " << ShapeUtil::HumanString(result_shape)
        << " but is inferred to be: "
        << ShapeUtil::HumanString(inferred_return_shape);
    TF_RET_CHECK(
        primitive_util::IsIntegralType(start_indices->shape().element_type()));

    const Literal& operand_literal = parent_->GetEvaluatedLiteralFor(operand);
    const Literal& start_indices_literal =
        parent_->GetEvaluatedLiteralFor(start_indices);

    switch (start_indices->shape().element_type()) {
      case S32: {
        TF_ASSIGN_OR_RETURN(
            parent_->evaluated_[dynamic_slice],
            DynamicSlice<int32>(operand_literal, start_indices_literal,
                                result_shape));
      } break;
      case S64: {
        TF_ASSIGN_OR_RETURN(
            parent_->evaluated_[dynamic_slice],
            DynamicSlice<int64>(operand_literal, start_indices_literal,
                                result_shape));
      } break;
      case U32: {
        TF_ASSIGN_OR_RETURN(
            parent_->evaluated_[dynamic_slice],
            DynamicSlice<uint32>(operand_literal, start_indices_literal,
                                 result_shape));
      } break;
      case U64: {
        TF_ASSIGN_OR_RETURN(
            parent_->evaluated_[dynamic_slice],
            DynamicSlice<uint64>(operand_literal, start_indices_literal,
                                 result_shape));
      } break;
      default:
        LOG(FATAL) << "HandleDynamicSlice: unhandled primitive type for "
                      "start_indices: "
                   << PrimitiveType_Name(start_indices->shape().element_type());
    }

    return Status::OK();
  }

  Status HandleDynamicUpdateSlice(
      HloInstruction* dynamic_update_slice) override {
    auto operand = dynamic_update_slice->operand(0);
    auto update = dynamic_update_slice->operand(1);
    auto start_indices = dynamic_update_slice->operand(2);
    auto result_shape = dynamic_update_slice->shape();
    TF_ASSIGN_OR_RETURN(
        auto inferred_return_shape,
        ShapeInference::InferDynamicUpdateSliceShape(
            operand->shape(), update->shape(), start_indices->shape()));
    TF_RET_CHECK(ShapeUtil::Compatible(result_shape, inferred_return_shape))
        << "return shape is set to: " << ShapeUtil::HumanString(result_shape)
        << " but is inferred to be: "
        << ShapeUtil::HumanString(inferred_return_shape);
    TF_RET_CHECK(
        primitive_util::IsIntegralType(start_indices->shape().element_type()));
    TF_RET_CHECK(ShapeUtil::Compatible(result_shape, operand->shape()));

    const Literal& operand_literal = parent_->GetEvaluatedLiteralFor(operand);
    const Literal& update_literal = parent_->GetEvaluatedLiteralFor(update);
    const Literal& start_indices_literal =
        parent_->GetEvaluatedLiteralFor(start_indices);

    switch (start_indices->shape().element_type()) {
      case S32: {
        TF_ASSIGN_OR_RETURN(
            parent_->evaluated_[dynamic_update_slice],
            DynamicUpdateSlice<int32>(operand_literal, update_literal,
                                      start_indices_literal));
      } break;
      case S64: {
        TF_ASSIGN_OR_RETURN(
            parent_->evaluated_[dynamic_update_slice],
            DynamicUpdateSlice<int64>(operand_literal, update_literal,
                                      start_indices_literal));
      } break;
      case U32: {
        TF_ASSIGN_OR_RETURN(
            parent_->evaluated_[dynamic_update_slice],
            DynamicUpdateSlice<uint32>(operand_literal, update_literal,
                                       start_indices_literal));
      } break;
      case U64: {
        TF_ASSIGN_OR_RETURN(
            parent_->evaluated_[dynamic_update_slice],
            DynamicUpdateSlice<uint64>(operand_literal, update_literal,
                                       start_indices_literal));
      } break;
      default:
        LOG(FATAL) << "HandleDynamicUpdateSlice: unhandled primitive type for "
                      "start_indices: "
                   << PrimitiveType_Name(start_indices->shape().element_type());
    }

    return Status::OK();
  }

  template <typename NativeT>
  StatusOr<Literal> MapImpl(HloInstruction* map) {
    auto operands = map->operands();
    HloComputation* computation = map->to_apply();

    Literal result(map->shape());

    HloEvaluator embedded_evaluator(parent_->max_loop_iterations_);
    TF_RETURN_IF_ERROR(
        result.Populate<ReturnT>([&](absl::Span<const int64> multi_index) {
          std::vector<Literal> arg_literals;
          arg_literals.reserve(operands.size());

          // Construct scalar literal parameters to be passed to the map
          // computation.
          for (auto operand : operands) {
            const Literal& arg_literal =
                parent_->GetEvaluatedLiteralFor(operand);

            auto curr_val = arg_literal.Get<NativeT>(multi_index);
            auto curr_val_literal = LiteralUtil::CreateR0<NativeT>(curr_val);

            arg_literals.push_back(std::move(curr_val_literal));
          }

          Literal computed_result =
              embedded_evaluator.Evaluate<Literal>(*computation, arg_literals)
                  .ConsumeValueOrDie();
          // Clear visit states so that the we can use the evaluate again on
          // the same computation.
          embedded_evaluator.ResetVisitStates();

          return computed_result.Get<ReturnT>({});
        }));
    return std::move(result);
  }

  Status HandleMap(HloInstruction* map) override {
    switch (map->operand(0)->shape().element_type()) {
      case PRED: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map], MapImpl<bool>(map));
        break;
      }
      case U8: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map], MapImpl<uint8>(map));
        break;
      }
      case U32: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map], MapImpl<uint32>(map));
        break;
      }
      case U64: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map], MapImpl<uint64>(map));
        break;
      }
      case S8: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map], MapImpl<int8>(map));
        break;
      }
      case S32: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map], MapImpl<int32>(map));
        break;
      }
      case S64: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map], MapImpl<int64>(map));
        break;
      }
      case F16: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map],
                            MapImpl<Eigen::half>(map));
        break;
      }
      case F32: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map], MapImpl<float>(map));
        break;
      }
      case F64: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map], MapImpl<double>(map));
        break;
      }
      case C64: {
        TF_ASSIGN_OR_RETURN(parent_->evaluated_[map], MapImpl<complex64>(map));
        break;
      }
      default:
        LOG(FATAL) << "HandleMap: unhandled primitive type for "
                      "input operand: "
                   << PrimitiveType_Name(
                          map->operand(0)->shape().element_type());
    }

    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<
                !is_complex_t<NativeT>::value &&
                !std::is_same<NativeT, bool>::value>::type* = nullptr>
  Status HandleSort(HloInstruction* sort) {
    auto keys = sort->operand(0);
    TF_RET_CHECK(sort->operand_count() == 1)
        << "Typed visitor does not support key-value sort";

    const Literal& keys_literal = parent_->GetEvaluatedLiteralFor(keys);
    int64 sort_dim = sort->dimensions(0);
    int64 sort_dim_elements = keys->shape().dimensions(sort_dim);
    int64 rank = ShapeUtil::Rank(keys->shape());
    if (rank == 0) {
      // Nothing to sort.
      parent_->evaluated_[sort] = keys_literal.Clone();
      return Status::OK();
    }
    Literal result_literal(keys_literal.shape());
    std::vector<int64> zero_base(rank, 0);
    std::vector<int64> increment(rank, 1);
    increment[sort_dim] = sort_dim_elements;
    // Iterate through each dimension except 'sort_dim'.
    TF_RETURN_IF_ERROR(ShapeUtil::ForEachIndexWithStatus(
        keys->shape(), zero_base, AsInt64Slice(keys->shape().dimensions()),
        increment, [&](absl::Span<const int64> indices) -> StatusOr<bool> {
          // Extract a slice from the literal that corresponds to exactly the
          // row in dimension 'sort_dim'.
          std::vector<int64> limit_indices(indices.begin(), indices.end());
          std::for_each(limit_indices.begin(), limit_indices.end(),
                        [](int64& index) { ++index; });
          limit_indices[sort_dim] = sort_dim_elements;
          TF_ASSIGN_OR_RETURN(auto row_to_sort,
                              keys_literal.Slice(indices, limit_indices)
                                  .Reshape({sort_dim_elements}));
          const auto& row_data = row_to_sort.data<NativeT>();

          std::vector<NativeT> result_data(row_data.begin(), row_data.end());
          std::stable_sort(result_data.begin(), result_data.end(),
                           [](const NativeT& a, const NativeT& b) {
                             return SafeLess<NativeT>(a, b);
                           });
          Literal sorted_row(ShapeUtil::MakeShape(keys->shape().element_type(),
                                                  {sort_dim_elements}));
          sorted_row.PopulateR1(absl::Span<const NativeT>(result_data));
          std::vector<int64> slice_dimensions(rank, 1);
          slice_dimensions[sort_dim] = sort_dim_elements;
          TF_ASSIGN_OR_RETURN(auto sorted_row_reshaped,
                              sorted_row.Reshape(slice_dimensions));
          std::vector<int64> start_indices(rank, 0);
          TF_RETURN_IF_ERROR(result_literal.CopySliceFrom(
              sorted_row_reshaped, start_indices, indices, slice_dimensions));
          return true;
        }));
    parent_->evaluated_[sort] = std::move(result_literal);
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<is_complex_t<NativeT>::value ||
                                    std::is_same<NativeT, bool>::value>::type* =
                nullptr>
  Status HandleSort(HloInstruction* sort) {
    return UnsupportedTypeError(sort);
  }

  Status HandleSort(HloInstruction* sort) override {
    return HandleSort<ReturnT>(sort);
  }

  Status HandleReduce(HloInstruction* hlo) override {
    HloReduceInstruction* reduce = Cast<HloReduceInstruction>(hlo);
    int64 num_args = reduce->inputs().size();
    bool has_tuple_output = ShapeUtil::IsTuple(reduce->shape());
    absl::Span<const int64> dimensions(reduce->dimensions());
    HloComputation* function = reduce->to_apply();

    absl::InlinedVector<const Shape*, 1> operand_shapes;
    for (const HloInstruction* operand : reduce->operands()) {
      operand_shapes.push_back(&operand->shape());
    }
    TF_ASSIGN_OR_RETURN(auto inferred_return_shape,
                        ShapeInference::InferReduceShape(
                            operand_shapes,
                            /*dimensions_to_reduce=*/dimensions,
                            /*to_apply=*/function->ComputeProgramShape()));
    TF_RET_CHECK(ShapeUtil::Compatible(reduce->shape(), inferred_return_shape))
        << "return shape is set to: " << ShapeUtil::HumanString(reduce->shape())
        << " but is inferred to be: "
        << ShapeUtil::HumanString(inferred_return_shape);

    absl::InlinedVector<const Literal*, 1> arg_literals(num_args);
    absl::InlinedVector<const Literal*, 1> init_literals(num_args);
    for (int64 i = 0; i < num_args; ++i) {
      arg_literals[i] = &parent_->GetEvaluatedLiteralFor(reduce->inputs()[i]);
      VLOG(3) << "HandleReduce arg_literal: " << arg_literals[i]->ToString();
      init_literals[i] =
          &parent_->GetEvaluatedLiteralFor(reduce->init_values()[i]);
      VLOG(3) << "HandleReduce init_literal: " << init_literals[i]->ToString();
      TF_RET_CHECK(ShapeUtil::IsScalar(init_literals[i]->shape()));
    }

    // All args and results have the same dimensions, so pick an arbitrary one.
    const Shape& arg_shape = arg_literals[0]->shape();
    const Shape& result_shape = ShapeUtil::IsTuple(reduce->shape())
                                    ? reduce->shape().tuple_shapes(0)
                                    : reduce->shape();
    const auto arg_dimensions = AsInt64Slice(arg_shape.dimensions());
    std::vector<int64> arg_dim_steps(arg_dimensions.size());
    std::vector<int64> arg_dim_counts(arg_dimensions.size());
    for (const int64 dim : dimensions) {
      arg_dim_steps[dim] = 1;
      arg_dim_counts[dim] = arg_dimensions[dim];
    }

    // Map each dimension in the result to a dimension in arg that isn't
    // being reduced.
    std::vector<int64> result_to_arg_index;
    for (int64 i = 0; i < arg_dimensions.size(); ++i) {
      if (arg_dim_steps[i] == 0) {
        result_to_arg_index.push_back(i);
      }
    }

    HloEvaluator embedded_evaluator(parent_->max_loop_iterations_);
    absl::InlinedVector<Literal, 1> results(num_args);
    for (int64 i = 0; i < num_args; ++i) {
      results[i] = Literal(result_shape);
    }

    Status eval_status;
    // For each resulting dimension, calculate and assign computed values.
    // This is really wasteful when num_args > 1, since we re-run the
    // reduction num_args time. The alternative is to teach Populate() about
    // tuples, which we should probably do.
    absl::InlinedVector<ReturnT, 1> init_scalars(num_args);
    for (int i = 0; i < num_args; ++i) {
      init_scalars[i] = init_literals[i]->Get<ReturnT>({});
    }

    for (int64 input = 0; input < num_args; ++input) {
      TF_RETURN_IF_ERROR(results[input].Populate<ReturnT>(
          [&](absl::Span<const int64> multi_index) {
            if (!eval_status.ok()) {
              return init_scalars[input];
            }
            absl::InlinedVector<ReturnT, 1> result_values(init_scalars.begin(),
                                                          init_scalars.end());
            std::vector<int64> base(arg_dimensions.size());
            for (int64 i = 0; i < multi_index.size(); ++i) {
              base[result_to_arg_index[i]] = multi_index[i];
            }

            // When the reduction is addition of floats, accumulate in a double
            // for better precision. Also, avoid creating Literals for the
            // intermediate results; it's much faster.
            if (ShapeUtil::ElementIsFloating(init_literals[0]->shape()) &&
                IsScalarAdd(function)) {
              CHECK_EQ(num_args, 1);
              double computed_result = 0;
              auto func = [&](absl::Span<const int64> input_index) {
                computed_result +=
                    GetAsDouble<ReturnT>(*arg_literals[0], input_index);
                return true;
              };
              ShapeUtil::ForEachIndex(arg_literals[0]->shape(), base,
                                      arg_dim_counts, arg_dim_steps, func);
              return static_cast<ReturnT>(computed_result);
            }
            auto func =
                [&](absl::Span<const int64> input_index) -> StatusOr<bool> {
              absl::InlinedVector<ReturnT, 1> arg_values(num_args);
              for (int64 i = 0; i < num_args; ++i) {
                arg_values[i] = arg_literals[i]->Get<ReturnT>(input_index);
              }

              // Evaluate computation with specified literal operands.
              absl::InlinedVector<Literal, 1> embedded_operands;
              for (ReturnT value : result_values) {
                embedded_operands.push_back(
                    LiteralUtil::CreateR0<ReturnT>(value));
              }
              for (ReturnT value : arg_values) {
                embedded_operands.push_back(
                    LiteralUtil::CreateR0<ReturnT>(value));
              }
              absl::InlinedVector<Literal*, 1> embedded_operands_ptrs(
                  embedded_operands.size());
              std::transform(embedded_operands.begin(), embedded_operands.end(),
                             embedded_operands_ptrs.begin(),
                             [](Literal& literal) { return &literal; });

              TF_ASSIGN_OR_RETURN(Literal computed_result,
                                  embedded_evaluator.Evaluate<const Literal*>(
                                      *function, embedded_operands_ptrs));
              // Clear visit states so that we can use the evaluator again on
              // the same computation.
              embedded_evaluator.ResetVisitStates();
              // Assign computed result to result_val.
              if (!has_tuple_output) {
                result_values[0] = computed_result.Get<ReturnT>({});
              } else {
                for (int64 i = 0; i < num_args; ++i) {
                  result_values[i] = computed_result.Get<ReturnT>(
                      /*multi_index=*/{}, /*shape_index=*/{i});
                }
              }
              return true;
            };
            // Computes one element of the result, reducing all dimensions that
            // contribute to that element.
            eval_status = ShapeUtil::ForEachIndexWithStatus(
                arg_shape, base, arg_dim_counts, arg_dim_steps, func);
            return result_values[input];
          }));
    }
    if (!has_tuple_output) {
      parent_->evaluated_[reduce] = std::move(results[0]);
    } else {
      Literal tuple_result(reduce->shape());
      for (int64 i = 0; i < num_args; ++i) {
        TF_CHECK_OK(tuple_result.MoveFrom(std::move(results[i]), {i}));
      }
      parent_->evaluated_[reduce] = std::move(tuple_result);
    }
    return eval_status;
  }

  bool IsScalarAdd(HloComputation* computation) {
    HloInstruction* instruction = computation->root_instruction();
    if (instruction->opcode() == HloOpcode::kAdd &&
        computation->num_parameters() == 2) {
      const HloInstruction* lhs = instruction->operand(0);
      const HloInstruction* rhs = instruction->operand(1);
      return lhs->opcode() == HloOpcode::kParameter &&
             ShapeUtil::IsScalar(lhs->shape()) &&
             rhs->opcode() == HloOpcode::kParameter &&
             ShapeUtil::IsScalar(rhs->shape()) && lhs != rhs;
    }
    return false;
  }

  Status HandleSelectAndScatter(HloInstruction* select_and_scatter) override {
    auto operand = select_and_scatter->operand(0);
    auto source = select_and_scatter->operand(1);
    const Window& window = select_and_scatter->window();

    const Literal& init_literal =
        parent_->GetEvaluatedLiteralFor(select_and_scatter->operand(2));
    TF_RET_CHECK(ShapeUtil::IsScalar(init_literal.shape()));
    auto init_scalar = init_literal.Get<ReturnT>({});

    Literal result(select_and_scatter->shape());

    // Initialize result array with the init value.
    TF_RETURN_IF_ERROR(result.Populate<ReturnT>(
        [&](absl::Span<const int64> output_index) { return init_scalar; }));

    std::vector<int64> window_dimension_sizes;
    for (const auto& window_dimension : window.dimensions()) {
      window_dimension_sizes.push_back(window_dimension.size());
    }
    const Shape window_shape = ShapeUtil::MakeShape(
        operand->shape().element_type(), window_dimension_sizes);

    HloComputation* select = select_and_scatter->select();
    HloComputation* scatter = select_and_scatter->scatter();

    const Literal& operand_literal = parent_->GetEvaluatedLiteralFor(operand);
    const Literal& source_literal = parent_->GetEvaluatedLiteralFor(source);

    int64 rank = ShapeUtil::Rank(operand_literal.shape());

    HloEvaluator embedded_evaluator(parent_->max_loop_iterations_);
    DimensionVector source_index(rank, 0);

    // Used in the dual IterateThroughWindow lambdas below. Hoisted to avoid
    // dynamic memory allocations.
    auto curr_val_literal = LiteralUtil::CreateR0<ReturnT>(ReturnT());
    auto selected_val_literal = LiteralUtil::CreateR0<ReturnT>(ReturnT());
    auto source_literal_scatter = LiteralUtil::CreateR0<ReturnT>(ReturnT());
    auto scattered_literal = LiteralUtil::CreateR0<ReturnT>(ReturnT());
    do {
      // For each element in `source`, we place a window in `operand`. For each
      // window placement, we iterate inside the window twice:
      //
      // 1. Find the selected index by applying `select` function to all
      // elements. E.g., If the `select` function is GreaterEqual, the first
      // iteration through the window finds the biggest value and returns its
      // index.
      //
      // 2. Using the selected index, scatter value from `source` to result. We
      // do this by iterating through the window, and compare each index with
      // the selected index.
      absl::optional<ReturnT> selected_val;
      absl::optional<std::vector<int64>> selected_index;

      IterateThroughWindow(
          window_shape, window, operand_literal.shape(), source_index,
          [&](const std::vector<int64>& operand_index) {
            auto curr_val = operand_literal.Get<ReturnT>(operand_index);
            if (!selected_val) {
              selected_val = curr_val;
              selected_index = operand_index;
            }
            curr_val_literal.Set({}, curr_val);
            selected_val_literal.Set({}, *selected_val);
            Literal computed_result =
                embedded_evaluator
                    .Evaluate<const Literal*>(
                        *select, {&selected_val_literal, &curr_val_literal})
                    .ConsumeValueOrDie();
            bool selected = !computed_result.Get<bool>({});
            if (selected) {
              selected_val = curr_val;
              selected_index = operand_index;
            }
            embedded_evaluator.ResetVisitStates();
          });

      IterateThroughWindow(
          window_shape, window, operand_literal.shape(), source_index,
          [&](const std::vector<int64>& operand_index) {
            if (std::equal(operand_index.begin(), operand_index.end(),
                           selected_index->begin())) {
              auto source = source_literal.Get<ReturnT>(source_index);
              auto scattered = result.Get<ReturnT>(operand_index);
              source_literal_scatter.Set({}, source);
              scattered_literal.Set({}, scattered);
              Literal computed_result =
                  embedded_evaluator
                      .Evaluate<const Literal*>(
                          *scatter,
                          {&source_literal_scatter, &scattered_literal})
                      .ConsumeValueOrDie();
              result.Set(operand_index, computed_result.Get<ReturnT>({}));
              // Clear visit states so that the we can use the evaluator again
              // on the same computation.
              embedded_evaluator.ResetVisitStates();
            }
          });
    } while (
        IndexUtil::BumpIndices(source->shape(), absl::MakeSpan(source_index)));

    parent_->evaluated_[select_and_scatter] = std::move(result);
    return Status::OK();
  }

  Status HandleReduceWindow(HloInstruction* reduce_window) override {
    auto operand = reduce_window->operand(0);
    const Window& window = reduce_window->window();
    HloComputation* function = reduce_window->to_apply();
    TF_ASSIGN_OR_RETURN(
        auto inferred_return_shape,
        ShapeInference::InferReduceWindowShape(
            /*operand_shape=*/reduce_window->operand(0)->shape(),
            /*init_value=*/reduce_window->operand(1)->shape(), window,
            /*to_apply_shape=*/function->ComputeProgramShape()));
    TF_RET_CHECK(
        ShapeUtil::Compatible(reduce_window->shape(), inferred_return_shape))
        << "return shape is set to: "
        << ShapeUtil::HumanStringWithLayout(reduce_window->shape())
        << " but is inferred to be: "
        << ShapeUtil::HumanStringWithLayout(inferred_return_shape);

    const Literal& operand_literal =
        parent_->GetEvaluatedLiteralFor(reduce_window->operand(0));
    VLOG(3) << "HandleReduceWindow arg_literal: " << operand_literal.ToString();
    const Literal& init_literal =
        parent_->GetEvaluatedLiteralFor(reduce_window->operand(1));
    VLOG(3) << "HandleReduceWindow init_literal: " << init_literal.ToString();
    TF_RET_CHECK(ShapeUtil::IsScalar(init_literal.shape()));
    auto init_scalar = init_literal.Get<ReturnT>({});

    // Creates a Shape object from window, for iteration below.
    std::vector<int64> window_dimension_sizes;
    for (const auto& window_dimension : window.dimensions()) {
      window_dimension_sizes.push_back(window_dimension.size());
    }
    const Shape window_shape = ShapeUtil::MakeShape(
        operand->shape().element_type(), window_dimension_sizes);

    DimensionVector window_index(window.dimensions_size());
    DimensionVector operand_index(ShapeUtil::Rank(operand_literal.shape()));

    HloEvaluator embedded_evaluator(parent_->max_loop_iterations_);
    Literal result(reduce_window->shape());
    // For each resulting dimension, calculate and assign computed value.
    TF_RETURN_IF_ERROR(
        result.Populate<ReturnT>([&](absl::Span<const int64> output_index) {
          ReturnT result_val = init_scalar;

          std::fill(window_index.begin(), window_index.end(), 0);
          std::fill(operand_index.begin(), operand_index.end(), 0);

          IterateThroughWindow(
              window_shape, window, operand_literal.shape(), output_index,
              [&](const std::vector<int64>& operand_index) {
                auto curr_val = operand_literal.Get<ReturnT>(operand_index);

                // Evaluate computation with specified literal operands.
                const auto curr_val_literal =
                    LiteralUtil::CreateR0<ReturnT>(curr_val);
                const auto result_val_literal =
                    LiteralUtil::CreateR0<ReturnT>(result_val);
                Literal computed_result =
                    embedded_evaluator
                        .Evaluate<const Literal*>(
                            *function, {&result_val_literal, &curr_val_literal})
                        .ConsumeValueOrDie();

                // Clear visit states so that the we can use the evaluate again
                // on the same computation.
                embedded_evaluator.ResetVisitStates();

                result_val = computed_result.Get<ReturnT>({});
              });

          return result_val;
        }));

    parent_->evaluated_[reduce_window] = std::move(result);
    return Status::OK();
  }

  // Reshapes the scatter indices input to have a trailing degenerate `1`
  // dimension if necessary.  Hands over the ownership of the newly created
  // literal (if there is one) to `reshaped_indices`.
  StatusOr<std::reference_wrapper<const Literal>> ReshapedScatterIndices(
      int64 index_vector_dim, const Literal& indices,
      Literal* reshaped_indices) {
    if (indices.shape().dimensions_size() != index_vector_dim) {
      return std::cref(indices);
    }

    std::vector<int64> new_shape(indices.shape().dimensions().begin(),
                                 indices.shape().dimensions().end());
    new_shape.push_back(1);
    TF_ASSIGN_OR_RETURN(*reshaped_indices, indices.Reshape(new_shape));
    return std::cref(*reshaped_indices);
  }

  // Returns an ShapeUtil::IndexIterationSpace that iterates over the update
  // scatter dimensions while keeping the rest of the update dimensions clamped
  // to 0.
  ShapeUtil::IndexIterationSpace IterationSpaceForUpdateScatterIndices(
      const Shape& updates_shape, const ScatterDimensionNumbers& dim_numbers) {
    int64 updates_rank = updates_shape.dimensions_size();
    std::vector<int64> index_base(updates_rank, 0);
    std::vector<int64> index_count(updates_rank, 1);
    for (int64 i = 0; i < updates_rank; i++) {
      bool is_update_scatter_dim =
          !absl::c_binary_search(dim_numbers.update_window_dims(), i);
      if (is_update_scatter_dim) {
        index_count[i] = updates_shape.dimensions(i);
      }
    }
    return {std::move(index_base), std::move(index_count),
            std::vector<int64>(updates_rank, 1)};
  }

  // Return an ShapeUtil::IndexIterationSpace that iterates over the update
  // window dimensions while keeping the rest of the update dimensions clamped
  // to 0.
  ShapeUtil::IndexIterationSpace IterationSpaceForUpdateWindowIndices(
      const Shape& updates_shape, const ScatterDimensionNumbers& dim_numbers) {
    int64 updates_rank = updates_shape.dimensions_size();
    std::vector<int64> index_base(updates_rank, 0);
    std::vector<int64> index_count(updates_rank, 1);
    for (int64 i = 0; i < updates_rank; i++) {
      bool is_update_window_dim =
          absl::c_binary_search(dim_numbers.update_window_dims(), i);
      if (is_update_window_dim) {
        index_count[i] = updates_shape.dimensions(i);
      }
    }
    return {std::move(index_base), std::move(index_count),
            std::vector<int64>(updates_rank, 1)};
  }

  // This functor computes the contribution of scatter_indices to an input index
  // corresponding to an update index.  That is, given an update index I, it
  // picks out the scatter indices in I and uses them to look up a scatter
  // index, S, from the scatter indices tensor, and expands S into the input
  // space according to scatter_dims_to_operand_dims.
  //
  // This is similar to the class HloEvaluator::OutputGatherIndexToInputIndex
  // that does the corresponding function for Gather.
  class UpdateScatterIndexToInputIndex {
   public:
    // The constructor does some setup work that is amortized across all
    // iterations.
    explicit UpdateScatterIndexToInputIndex(
        const ScatterDimensionNumbers* dim_numbers, const Shape& input_shape,
        const Shape& updates_shape, const Literal* scatter_indices)
        : dim_numbers_(*dim_numbers), scatter_indices_(*scatter_indices) {
      for (int64 i = 0; i < updates_shape.dimensions_size(); i++) {
        update_dim_is_scatter_dims_.push_back(
            !absl::c_binary_search(dim_numbers_.update_window_dims(), i));
      }

      for (int64 i = 0; i < input_shape.dimensions_size(); i++) {
        int64 index_of_input_dim_in_index_vector =
            FindIndex(dim_numbers_.scatter_dims_to_operand_dims(), i);
        if (index_of_input_dim_in_index_vector ==
            dim_numbers_.scatter_dims_to_operand_dims_size()) {
          input_dim_value_to_index_vector_.push_back(-1);
        } else {
          input_dim_value_to_index_vector_.push_back(
              index_of_input_dim_in_index_vector);
        }
      }

      index_vector_index_.resize(scatter_indices_.shape().dimensions_size());
      input_index_.resize(input_shape.dimensions_size());
      int64 index_vector_size =
          scatter_indices_.shape().dimensions(dim_numbers_.index_vector_dim());
      index_vector_.resize(index_vector_size);
    }

    // Returns the contribution of scatter_indices to the input index
    // corresponding to update_index.  See scatter_inner_loop_body.
    //
    // This is conceptually  a stateless transformation from update_index to the
    // scatter input index, but:
    //
    //  - Instead of allocating memory to represent the scatter input index on
    //    every invocation we reuse the same storage for the result
    //    (input_index_), mutating it in place.
    //  - Instead of allocating buffers for temporary values like
    //    index_vector_index_ and index_vector on every invocation, we reuse the
    //    same storage for all invocations.
    //
    // This returns a Span into memory owned by the class.
    StatusOr<absl::Span<const int64>> operator()(
        absl::Span<const int64> update_index) {
      PropagateUpdateIndexScatterDimsToIndexVectorIndex(update_index);
      TF_RETURN_IF_ERROR(FetchIndexVector());
      PropagateIndexVectorToInputIndex();
      return absl::Span<const int64>(input_index_);
    }

   private:
    // Propagates the scatter index dimensions from the update index into
    // index_vector_index_ by mutating index_vector_index_ in place.  Does not
    // update the dim_numbers.index_vector_dim() dimension -- that's the
    // dimension we iterate over in FetchIndexVector.
    void PropagateUpdateIndexScatterDimsToIndexVectorIndex(
        absl::Span<const int64> update_index) {
      int64 index_vector_index_i = 0;
      for (int64 i = 0, e = update_index.size(); i < e; i++) {
        if (!update_dim_is_scatter_dims_[i]) {
          continue;
        }

        if (index_vector_index_i == dim_numbers_.index_vector_dim()) {
          index_vector_index_i++;
        }

        index_vector_index_[index_vector_index_i++] = update_index[i];
      }
    }

    // Populates index_vector_ by iterating over scatter_indices_ according to
    // index_vector_index_.
    Status FetchIndexVector() {
      int64 index_vector_dim = dim_numbers_.index_vector_dim();
      for (int64 i = 0, e = index_vector_.size(); i < e; i++) {
        index_vector_index_[index_vector_dim] = i;
        TF_ASSIGN_OR_RETURN(index_vector_[i], scatter_indices_.GetIntegralAsS64(
                                                  index_vector_index_));
      }
      return Status::OK();
    }

    // Populates input_index_.
    void PropagateIndexVectorToInputIndex() {
      for (int64 i = 0, e = input_index_.size(); i < e; i++) {
        if (input_dim_value_to_index_vector_[i] != -1) {
          input_index_[i] = index_vector_[input_dim_value_to_index_vector_[i]];
        }

        // If input_dim_value_to_index_vector_[i] == -1 then input_index_[i]
        // remains 0, as set by the constructor.
      }
    }

    // input_dim_value_to_index_vector_[i] tells us how to compute dimension i
    // of the input index from the index vector.  See
    // PropagateIndexVectorToInputIndex.
    std::vector<int64> input_dim_value_to_index_vector_;

    // update_dim_is_scatter_dims_[i] is true iff the update index i is a
    // scatter dimension.
    std::vector<bool> update_dim_is_scatter_dims_;

    // The buffer into which we construct an index into scatter_indices_ to
    // fetch the index vector.
    std::vector<int64> index_vector_index_;

    // The index vector fetched from scatter_indices_.
    std::vector<int64> index_vector_;

    // The result computed by this functor.  operator() returns a Span
    // into this vector.
    std::vector<int64> input_index_;

    const ScatterDimensionNumbers& dim_numbers_;
    const Literal& scatter_indices_;
  };

  // This functor computes the contribution of the window indices in an update
  // index to an input index.  That is, given an update index I it picks out the
  // update window indices in I and expands it into a window index into the
  // input shape.
  //
  // This is similar to the class HloEvaluator::OutputWindowIndexToInputIndex
  // that does the corresponding function for Gather.
  class UpdateWindowIndexToInputIndex {
   public:
    // The constructor does some setup work that is amortized across all
    // iterations.
    explicit UpdateWindowIndexToInputIndex(
        const ScatterDimensionNumbers& dim_numbers, const Shape& input_shape,
        const Shape& updates_shape) {
      std::vector<int64> window_index_to_update_index;
      int64 update_index_count = 0;
      for (int64 i = 0; i < updates_shape.dimensions_size(); i++) {
        if (absl::c_binary_search(dim_numbers.update_window_dims(), i)) {
          window_index_to_update_index.push_back(update_index_count++);
        } else {
          update_index_count++;
        }
      }

      int64 window_dim_count = 0;
      for (int64 i = 0; i < input_shape.dimensions_size(); i++) {
        if (absl::c_binary_search(dim_numbers.inserted_window_dims(), i)) {
          input_dim_value_to_update_index_.push_back(-1);
        } else {
          input_dim_value_to_update_index_.push_back(
              window_index_to_update_index[window_dim_count++]);
        }
      }

      input_index_.resize(input_shape.dimensions_size());
    }

    // Returns the contribution of the window indices to the input index
    // corresponding to update_index.  See scatter_inner_loop_body.
    //
    // This is conceptually a stateless transformation from update_index to the
    // window input index, but instead of allocating memory to represent the
    // scatter input index on every invocation we reuse the same storage for the
    // result (input_index_), mutating it in place.
    //
    // This returns a Span into memory owned by the class.
    StatusOr<absl::Span<const int64>> operator()(
        absl::Span<const int64> update_index) {
      PropagateUpdateIndexWindowDimsToInputIndex(update_index);
      return absl::Span<const int64>(input_index_);
    }

    // Returns for a given 'input_dim' the corresponding update dimension index,
    // or -1 if 'input_dim' is an elided window dimension.
    int64 input_dim_value_to_update_index(int64 input_dim) {
      return input_dim_value_to_update_index_[input_dim];
    }

   private:
    // Propagates window dimensions from the update index to input_index_ by
    // mutating input_index_ in place.
    void PropagateUpdateIndexWindowDimsToInputIndex(
        absl::Span<const int64> update_index) {
      for (int64 i = 0, e = input_index_.size(); i < e; i++) {
        if (input_dim_value_to_update_index_[i] != -1) {
          input_index_[i] = update_index[input_dim_value_to_update_index_[i]];
        }

        // If input_dim_value_to_index_vector_[i] == -1 then input_index_[i]
        // remains 0, as set by the constructor.
      }
    }

    // input_dim_value_to_index_vector_[i] tells us how to compute dimension i
    // of the input index from the update index. See
    // PropagateUpdateIndexWindowDimsToInputIndex.
    std::vector<int64> input_dim_value_to_update_index_;

    // The result computed by this functor.  operator() returns a Span
    // into this vector.
    std::vector<int64> input_index_;
  };

  Status HandleScatter(HloInstruction* scatter) override {
    const ScatterDimensionNumbers& dim_numbers =
        scatter->scatter_dimension_numbers();
    const Literal& operand =
        parent_->GetEvaluatedLiteralFor(scatter->operand(0));
    Literal reshaped_scatter_indices;
    TF_ASSIGN_OR_RETURN(const Literal& scatter_indices,
                        ReshapedScatterIndices(dim_numbers.index_vector_dim(),
                                               parent_->GetEvaluatedLiteralFor(
                                                   scatter->operand(1)),
                                               &reshaped_scatter_indices));
    const Literal& updates =
        parent_->GetEvaluatedLiteralFor(scatter->operand(2));
    const Shape& updates_shape = updates.shape();
    const Shape& operand_shape = operand.shape();

    ShapeUtil::IndexIterationSpace scatter_indices_iteration_space =
        IterationSpaceForUpdateScatterIndices(updates_shape, dim_numbers);
    ShapeUtil::IndexIterationSpace window_indices_iteration_space =
        IterationSpaceForUpdateWindowIndices(updates_shape, dim_numbers);

    std::vector<int64> input_index(operand_shape.dimensions_size());
    std::vector<int64> update_index(updates_shape.dimensions_size());
    std::vector<int64> input_scatter_index_clamped(
        operand_shape.dimensions_size());

    UpdateScatterIndexToInputIndex update_scatter_index_to_input_index(
        &scatter->scatter_dimension_numbers(), /*input_shape=*/operand_shape,
        updates_shape, &scatter_indices);
    UpdateWindowIndexToInputIndex update_window_index_to_input_index(
        scatter->scatter_dimension_numbers(), /*input_shape=*/operand_shape,
        updates_shape);

    // Initialize the result with the operand. This makes it easier to handle
    // the updates even when the indices are repeated.
    Literal result = operand.Clone();
    HloEvaluator embedded_evaluator;
    auto scatter_inner_loop_body =
        [&](absl::Span<const int64> update_window_index,
            absl::Span<const int64> input_scatter_index,
            absl::Span<const int64> update_scatter_index) -> StatusOr<bool> {
      TF_ASSIGN_OR_RETURN(
          absl::Span<const int64> input_window_index,
          update_window_index_to_input_index(update_window_index));
      for (int i = 0, e = update_index.size(); i < e; i++) {
        update_index[i] = update_scatter_index[i] + update_window_index[i];
        DCHECK_LT(update_index[i], updates_shape.dimensions(i));
      }
      for (int i = 0, e = input_scatter_index.size(); i < e; i++) {
        int64 update_dim =
            update_window_index_to_input_index.input_dim_value_to_update_index(
                i);
        // If 'update_dim' is -1, it means 'i' is an elided window dim. This
        // means we set the iteration index to 0, so for the purpose of the
        // following calculations we can consider the update dimension size to
        // be 1.
        int64 update_dim_size =
            update_dim == -1 ? 1 : updates_shape.dimensions(update_dim);
        // If any part of the update region is out-of-bounds, then do not
        // perform any update on the input.
        if ((input_scatter_index[i] < 0) ||
            (input_scatter_index[i] >
             operand_shape.dimensions(i) - update_dim_size)) {
          return true;
        }
      }
      for (int i = 0, e = input_index.size(); i < e; i++) {
        input_index[i] = input_scatter_index[i] + input_window_index[i];
      }

      auto result_value_literal =
          LiteralUtil::CreateR0<ReturnT>(result.Get<ReturnT>(input_index));
      auto update_value_literal =
          LiteralUtil::CreateR0<ReturnT>(updates.Get<ReturnT>(update_index));
      Literal updated_result =
          embedded_evaluator
              .Evaluate<const Literal*>(
                  *scatter->to_apply(),
                  {&result_value_literal, &update_value_literal})
              .ConsumeValueOrDie();
      // Clear visit states so that the we can use the evaluate again on the
      // same computation.
      embedded_evaluator.ResetVisitStates();
      result.Set<ReturnT>(input_index, updated_result.Get<ReturnT>({}));
      return true;
    };

    auto scatter_outer_loop_body =
        [&](absl::Span<const int64> update_scatter_index) -> StatusOr<bool> {
      TF_ASSIGN_OR_RETURN(
          absl::Span<const int64> input_scatter_index,
          update_scatter_index_to_input_index(update_scatter_index));
      TF_RETURN_IF_ERROR(ShapeUtil::ForEachIndexWithStatus(
          updates_shape, window_indices_iteration_space,
          [&](absl::Span<const int64> update_window_index) {
            return scatter_inner_loop_body(
                update_window_index, input_scatter_index, update_scatter_index);
          }));
      return true;
    };

    TF_RETURN_IF_ERROR(ShapeUtil::ForEachIndexWithStatus(
        updates_shape, scatter_indices_iteration_space,
        scatter_outer_loop_body));
    parent_->evaluated_[scatter] = std::move(result);
    return Status::OK();
  }

  Status HandleSlice(HloInstruction* slice) override {
    auto operand = slice->operand(0);
    const Shape& shape = slice->shape();
    TF_ASSIGN_OR_RETURN(auto inferred_return_shape,
                        ShapeInference::InferSliceShape(
                            operand->shape(), slice->slice_starts(),
                            slice->slice_limits(), slice->slice_strides()));
    TF_RET_CHECK(ShapeUtil::Compatible(shape, inferred_return_shape))
        << "return shape set to: " << ShapeUtil::HumanString(shape)
        << " but is inferred to be: "
        << ShapeUtil::HumanString(inferred_return_shape);

    const int64 rank = ShapeUtil::Rank(operand->shape());
    const Literal& operand_literal = parent_->GetEvaluatedLiteralFor(operand);
    auto func = [&](absl::Span<const int64> out_index) {
      DimensionVector operand_index(rank);
      for (int64 i = 0; i < rank; ++i) {
        operand_index[i] =
            slice->slice_starts(i) + out_index[i] * slice->slice_strides(i);
      }
      return operand_literal.Get<ReturnT>(operand_index);
    };

    Literal result(shape);
    TF_RETURN_IF_ERROR(result.Populate<ReturnT>(func));
    parent_->evaluated_[slice] = std::move(result);
    return Status::OK();
  }

  // Enable CLZ only for int32, uint32, int64 and uint64.
  template <
      typename NativeT,
      typename std::enable_if<
          (std::is_floating_point<NativeT>::value ||
           std::is_integral<NativeT>::value || is_complex_t<NativeT>::value) &&
          !(std::is_same<NativeT, uint32>::value ||
            std::is_same<NativeT, int32>::value ||
            std::is_same<NativeT, int64>::value ||
            std::is_same<NativeT, uint64>::value)>::type* = nullptr>
  Status HandleClz(HloInstruction* clz) {
    return UnsupportedTypeError(clz);
  }

  template <typename NativeT,
            typename std::enable_if<
                std::is_same<NativeT, uint32>::value ||
                std::is_same<NativeT, int32>::value>::type* = nullptr>
  Status HandleClz(HloInstruction* clz) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[clz],
                        ElementWiseUnaryOp(clz, [](ElementwiseT elem_operand) {
                          return 31 - tensorflow::Log2Floor(elem_operand);
                        }));
    return Status::OK();
  }

  template <typename NativeT,
            typename std::enable_if<
                std::is_same<NativeT, uint64>::value ||
                std::is_same<NativeT, int64>::value>::type* = nullptr>
  Status HandleClz(HloInstruction* clz) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[clz],
                        ElementWiseUnaryOp(clz, [](ElementwiseT elem_operand) {
                          return 63 - tensorflow::Log2Floor64(elem_operand);
                        }));
    return Status::OK();
  }

  Status HandleClz(HloInstruction* clz) override {
    return HandleClz<ElementwiseT>(clz);
  }

  template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleSin(HloInstruction* sin) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[sin],
                        ElementWiseUnaryOp(sin, [](ElementwiseT elem_operand) {
                          return std::sin(elem_operand);
                        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<std::is_integral<NativeT>::value ||
                              is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleSin(HloInstruction* sin) {
    return UnsupportedTypeError(sin);
  }

  Status HandleSin(HloInstruction* sin) override {
    return HandleSin<ElementwiseT>(sin);
  }

  template <typename NativeT, typename std::enable_if<std::is_floating_point<
                                  NativeT>::value>::type* = nullptr>
  Status HandleCos(HloInstruction* cos) {
    TF_ASSIGN_OR_RETURN(parent_->evaluated_[cos],
                        ElementWiseUnaryOp(cos, [](ElementwiseT elem_operand) {
                          return std::cos(elem_operand);
                        }));
    return Status::OK();
  }

  template <
      typename NativeT,
      typename std::enable_if<std::is_integral<NativeT>::value ||
                              is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleCos(HloInstruction* cos) {
    return UnsupportedTypeError(cos);
  }

  Status HandleCos(HloInstruction* cos) override {
    return HandleCos<ElementwiseT>(cos);
  }

  template <typename NativeT, typename std::enable_if<std::is_same<
                                  float, NativeT>::value>::type* = nullptr>
  Status HandleReducePrecision(HloInstruction* reduce_precision) {
    TF_ASSIGN_OR_RETURN(
        parent_->evaluated_[reduce_precision],
        ElementWiseUnaryOp(reduce_precision, [reduce_precision](
                                                 ElementwiseT elem) {
          uint32_t value_as_int = absl::bit_cast<uint32_t>(elem);
          const uint32_t mantissa_bits = reduce_precision->mantissa_bits();
          const uint32_t exponent_bits = reduce_precision->exponent_bits();

          // Code is based on the CPU/GPU implementation in LLVM-emitting code.
          //
          // Bits in float type:
          //   mantissa : bits [0:22]
          //   exponent : bits [23:30]
          //   sign     : bits [31]
          if (mantissa_bits < 23) {
            const uint32_t last_mantissa_bit_mask = 1u << (23 - mantissa_bits);

            // Compute rounding bias for round-to-nearest with ties to even.
            // This is equal to a base value of 0111... plus one bit if the last
            // remaining mantissa bit is 1.
            const uint32_t base_rounding_bias =
                (last_mantissa_bit_mask >> 1) - 1;
            const uint32_t x_last_mantissa_bit =
                (value_as_int & last_mantissa_bit_mask) >> (23 - mantissa_bits);
            const uint32_t x_rounding_bias =
                x_last_mantissa_bit + base_rounding_bias;

            // Add rounding bias, and mask out truncated bits.  Note that the
            // case where adding the rounding bias overflows into the exponent
            // bits is correct; the non-masked mantissa bits will all be zero,
            // and the exponent will be incremented by one.
            const uint32_t truncation_mask = ~(last_mantissa_bit_mask - 1);
            value_as_int = value_as_int + x_rounding_bias;
            value_as_int = value_as_int & truncation_mask;
          }
          if (exponent_bits < 8) {
            // Masks for f32 values.
            const uint32_t f32_sign_bit_mask = 1u << 31;
            const uint32_t f32_exp_bits_mask = 0xffu << 23;

            // An exponent of 2^(n-1)-1 -- that is, 0111... with the zero in the
            // most- significant bit -- is equal to 1.0f for all exponent sizes.
            // Adding 2^(n-1)-1 to this gives us the highest non-infinite
            // exponent for a bit- size of n, and subtracting 2^(n-1)-1 from
            // this gives us the lowest' exponent (corresponding to 0.0f).
            //
            // Thus, the f32 exponent corresponding to the highest non-infinite
            // exponent for a bit size of n is (2^7-1) + 2^(n-1)-1, and the f32
            // exponent corresponding to the lowest exponent for a bit size of n
            // is (2^7-1) - 2^(n-1)-1.
            //
            // Note that we have already checked that exponents_bits >= 1.
            const uint32_t f32_exponent_bias = (1 << 7) - 1;
            const uint32_t reduced_exponent_bias =
                (1 << (exponent_bits - 1)) - 1;
            const uint32_t reduced_max_exponent =
                f32_exponent_bias + reduced_exponent_bias;
            const uint32_t reduced_min_exponent =
                f32_exponent_bias - reduced_exponent_bias;

            // Do we overflow or underflow?
            const uint32_t x_exponent = value_as_int & f32_exp_bits_mask;
            const bool x_overflows = x_exponent > (reduced_max_exponent << 23);
            const bool x_underflows =
                x_exponent <= (reduced_min_exponent << 23);

            // Compute appropriately-signed values of zero and infinity.
            const uint32_t x_signed_zero = value_as_int & f32_sign_bit_mask;
            const uint32_t x_signed_inf = x_signed_zero | f32_exp_bits_mask;

            // Force to zero or infinity if overflow or underflow.  (Note that
            // this truncates all denormal values to zero, rather than rounding
            // them.)
            value_as_int = x_overflows ? x_signed_inf : value_as_int;
            value_as_int = x_underflows ? x_signed_zero : value_as_int;
          }

          float reduced_result = absl::bit_cast<float>(value_as_int);
          if (std::isnan(elem)) {
            reduced_result = mantissa_bits > 0
                                 ? elem
                                 : std::numeric_limits<float>::infinity();
          }
          return reduced_result;
        }));
    return Status::OK();
  }

  template <typename NativeT, typename std::enable_if<std::is_same<
                                  double, NativeT>::value>::type* = nullptr>
  Status HandleReducePrecision(HloInstruction* reduce_precision) {
    return InvalidArgument("Double not supported for reduce precision");
  }

  template <
      typename NativeT,
      typename std::enable_if<std::is_integral<NativeT>::value ||
                              is_complex_t<NativeT>::value>::type* = nullptr>
  Status HandleReducePrecision(HloInstruction* reduce_precision) {
    return UnsupportedTypeError(reduce_precision);
  }

  Status HandleReducePrecision(HloInstruction* reduce_precision) override {
    return HandleReducePrecision<ElementwiseT>(reduce_precision);
  }

  template <typename NativeT,
            typename std::enable_if<
                std::is_same<NativeT, bfloat16>::value ||
                std::is_same<NativeT, Eigen::half>::value ||
                std::is_integral<NativeT>::value ||
                std::is_floating_point<NativeT>::value>::type* = nullptr>
  Status HandleIota(HloInstruction* instruction) {
    auto* iota = Cast<HloIotaInstruction>(instruction);
    const int64 iota_size = iota->shape().dimensions(iota->iota_dimension());
    // Avoid using std::vector since std::vector<bool> does not convert to
    // absl::Span<bool>.
    absl::InlinedVector<NativeT, 1> data(iota_size);
    // We don't use std::iota for two reasons:
    //
    // (1) std:iota does not support bfloat16 and float16.
    //
    // (2) std::iota saturates for floating point types when the value is not
    //     representable, but the definition of HLO iota is the value as a
    //     64-bit integer cast to the native type.
    for (int64 i = 0; i < iota_size; ++i) {
      // static_cast is required for Eigen::half (F16).
      data[i] = static_cast<NativeT>(i);
    }
    auto result = LiteralUtil::CreateR1<NativeT>(data);

    if (ShapeUtil::Rank(iota->shape()) > 1) {
      TF_ASSIGN_OR_RETURN(
          parent_->evaluated_[iota],
          result.Broadcast(iota->shape(), {iota->iota_dimension()}));
    } else {
      TF_RET_CHECK(ShapeUtil::Rank(iota->shape()) == 1);
      parent_->evaluated_[iota] = std::move(result);
    }

    return Status::OK();
  }
  template <typename NativeT,
            typename std::enable_if<
                !(std::is_same<NativeT, bfloat16>::value ||
                  std::is_same<NativeT, Eigen::half>::value ||
                  std::is_integral<NativeT>::value ||
                  std::is_floating_point<NativeT>::value)>::type* = nullptr>
  Status HandleIota(HloInstruction* iota) {
    return UnsupportedTypeError(iota);
  }
  Status HandleIota(HloInstruction* iota) override {
    return HandleIota<ReturnT>(iota);
  }

 private:
  // Creates a vector of multipliers which can be used to create a linear index
  // into shape.
  //
  // Given the multidimensional index {i1, ..., iN} and
  // M = MakeDimMultipliers(shape), the corresponding linear index LI is simply
  //
  //   LI = i1 * M[1] + i2 * M[2] + ... + iN * M[N].
  //
  // This lets you calculate LI given the multidimensional indices in any order.
  static DimensionVector MakeDimMultipliers(const Shape& shape) {
    DimensionVector v(ShapeUtil::Rank(shape));
    int64 scale = 1;
    for (auto dim : LayoutUtil::MinorToMajor(shape)) {
      v[dim] = scale;
      scale *= shape.dimensions(dim);
    }
    return v;
  }

  // For one particular placement of a window in a base shape (the placement is
  // represented as `window_count_index`), iterates inside the window.
  // Translates the window index into base index. If the base index is within
  // bound, call `f` with the base index.
  static void IterateThroughWindow(
      const Shape& window_shape, const Window& window, const Shape& base_shape,
      const absl::Span<const int64>& window_count_index,
      const std::function<void(const std::vector<int64>&)>& f) {
    const int64 rank = ShapeUtil::Rank(base_shape);
    DimensionVector window_index(rank);
    std::fill(window_index.begin(), window_index.end(), 0);
    do {
      std::vector<int64> base_index(rank);
      bool out_of_bound = false;
      for (int64 i = 0; i < rank; ++i) {
        base_index[i] =
            window_count_index[i] * window.dimensions(i).stride() +
            window_index[i] * window.dimensions(i).window_dilation() -
            window.dimensions(i).padding_low();
        // We are not in the base area if the dilation placed us out of bounds.
        if (base_index[i] % window.dimensions(i).base_dilation() != 0) {
          out_of_bound = true;
          break;
        }
        // Apply the dilation to the base area.
        base_index[i] /= window.dimensions(i).base_dilation();
        if (base_index[i] < 0 || base_index[i] >= base_shape.dimensions(i)) {
          out_of_bound = true;
          break;
        }
      }
      if (!out_of_bound) {
        f(base_index);
      }
    } while (
        IndexUtil::BumpIndices(window_shape, absl::MakeSpan(window_index)));
  }

  template <typename IndexT>
  StatusOr<Literal> DynamicSlice(const Literal& operand_literal,
                                 const Literal& start_indices_literal,
                                 const Shape& result_shape) {
    auto start_indices_typed = start_indices_literal.data<IndexT>();
    std::vector<int64> start(start_indices_typed.begin(),
                             start_indices_typed.end());

    // Clamp the start indices so the slice is in-bounds w.r.t the operand.
    for (int64 i = 0; i < start.size(); ++i) {
      start[i] = std::min<int64>(
          std::max(int64{0}, start[i]),
          operand_literal.shape().dimensions(i) - result_shape.dimensions(i));
    }

    std::vector<int64> operand_indices(start.size());
    Literal result(result_shape);
    TF_RETURN_IF_ERROR(
        result.Populate<ReturnT>([&](absl::Span<const int64> multi_index) {
          for (int64 i = 0; i < operand_indices.size(); ++i) {
            CHECK_GE(multi_index[i] + start[i], 0);
            operand_indices[i] = multi_index[i] + start[i];
          }

          auto result = operand_literal.Get<ReturnT>(operand_indices);
          return result;
        }));

    return std::move(result);
  }

  template <typename IndexT>
  StatusOr<Literal> DynamicUpdateSlice(const Literal& operand_literal,
                                       const Literal& update_literal,
                                       const Literal& start_indices_literal) {
    auto result = operand_literal.Clone();
    auto start_indices_typed = start_indices_literal.data<IndexT>();
    const auto rank = ShapeUtil::Rank(result.shape());
    std::vector<int64> start(start_indices_typed.begin(),
                             start_indices_typed.end());
    // Clamp the update start indices so the slice is in-bounds w.r.t the
    // operand.
    for (int64 i = 0; i < rank; ++i) {
      start[i] = std::min<int64>(
          std::max<int64>(0, start[i]),
          result.shape().dimensions(i) - update_literal.shape().dimensions(i));
    }
    std::vector<int64> result_index(rank, 0);

    auto func = [&](absl::Span<const int64> update_index) {
      std::transform(update_index.begin(), update_index.end(), start.begin(),
                     result_index.begin(), std::plus<int64>());
      result.Set<ReturnT>(result_index,
                          update_literal.Get<ReturnT>(update_index));
      return true;
    };

    std::vector<int64> base(update_literal.shape().dimensions_size(), 0);
    std::vector<int64> step(update_literal.shape().dimensions_size(), 1);
    ShapeUtil::ForEachIndex(update_literal.shape(), base,
                            AsInt64Slice(update_literal.shape().dimensions()),
                            step, func);

    return std::move(result);
  }

  StatusOr<Literal> ElementWiseUnaryOp(
      HloInstruction* instruction,
      const std::function<ElementwiseT(ElementwiseT)>& unary_op) {
    const Literal& operand_literal =
        parent_->GetEvaluatedLiteralFor(instruction->operand(0));
    TF_ASSIGN_OR_RETURN(
        auto result_literal,
        (HloEvaluator::ElementWiseUnaryOpImpl<ReturnT, ReturnT>(
            instruction, ConvertUnaryFunction(unary_op), operand_literal)));

    return std::move(result_literal);
  }

  StatusOr<Literal> ElementWiseBinaryOp(
      HloInstruction* instruction,
      const std::function<ElementwiseT(ElementwiseT, ElementwiseT)>&
          binary_op) {
    const auto shape = instruction->shape();
    const auto* lhs = instruction->operand(0);
    const auto* rhs = instruction->operand(1);
    TF_RET_CHECK(ShapeUtil::SameDimensions(shape, rhs->shape()));
    TF_RET_CHECK(ShapeUtil::SameDimensions(lhs->shape(), rhs->shape()));

    const Literal& lhs_literal = parent_->GetEvaluatedLiteralFor(lhs);
    const Literal& rhs_literal = parent_->GetEvaluatedLiteralFor(rhs);

    Literal result(shape);

    TF_RETURN_IF_ERROR(
        result.Populate<ReturnT>([&](absl::Span<const int64> multi_index) {
          return ConvertBinaryFunction(binary_op)(
              lhs_literal.Get<ReturnT>(multi_index),
              rhs_literal.Get<ReturnT>(multi_index));
        }));
    return std::move(result);
  }

  template <typename LhsType, typename RhsType, typename EhsType>
  StatusOr<Literal> ElementwiseTernaryOp(
      HloInstruction* instruction,
      const std::function<ReturnT(LhsType, RhsType, EhsType)>& ternary_op) {
    const auto shape = instruction->shape();
    const auto* lhs = instruction->operand(0);
    const auto* rhs = instruction->operand(1);
    const auto* ehs = instruction->operand(2);
    TF_RET_CHECK(ShapeUtil::SameDimensions(shape, lhs->shape()));
    TF_RET_CHECK(ShapeUtil::SameDimensions(lhs->shape(), rhs->shape()));
    TF_RET_CHECK(ShapeUtil::SameDimensions(rhs->shape(), ehs->shape()));

    const Literal& lhs_literal = parent_->GetEvaluatedLiteralFor(lhs);
    const Literal& rhs_literal = parent_->GetEvaluatedLiteralFor(rhs);
    const Literal& ehs_literal = parent_->GetEvaluatedLiteralFor(ehs);

    Literal result(shape);

    TF_RETURN_IF_ERROR(
        result.Populate<ReturnT>([&](absl::Span<const int64> multi_index) {
          return ternary_op(lhs_literal.Get<LhsType>(multi_index),
                            rhs_literal.Get<RhsType>(multi_index),
                            ehs_literal.Get<EhsType>(multi_index));
        }));

    return std::move(result);
  }

  template <typename NativeT>
  static bool IsShiftOutOfBounds(NativeT rhs) {
    typedef typename std::make_unsigned<NativeT>::type UnsignedT;
    UnsignedT lhs_size_unsigned = sizeof(NativeT) * CHAR_BIT;
    UnsignedT rhs_unsigned = static_cast<UnsignedT>(rhs);
    return rhs_unsigned >= lhs_size_unsigned;
  }

  HloEvaluator* parent_;
};

// These extern templates prevent users of this class from implicitly
// instantiating it.  We explicitly instantiate this class in the various
// hlo_evaluator_typed_visitor*.cc files.
extern template class HloEvaluatorTypedVisitor<bool>;
extern template class HloEvaluatorTypedVisitor<uint8>;
extern template class HloEvaluatorTypedVisitor<uint32>;
extern template class HloEvaluatorTypedVisitor<uint64>;
extern template class HloEvaluatorTypedVisitor<int8>;
extern template class HloEvaluatorTypedVisitor<int32>;
extern template class HloEvaluatorTypedVisitor<int64>;
extern template class HloEvaluatorTypedVisitor<Eigen::half, float>;
extern template class HloEvaluatorTypedVisitor<float>;
extern template class HloEvaluatorTypedVisitor<double>;
extern template class HloEvaluatorTypedVisitor<complex64>;
extern template class HloEvaluatorTypedVisitor<bfloat16, float>;

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_HLO_EVALUATOR_TYPED_VISITOR_H_
