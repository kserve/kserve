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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_HLO_CREATION_UTILS_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_HLO_CREATION_UTILS_H_

#include "tensorflow/compiler/xla/literal_util.h"
#include "tensorflow/compiler/xla/service/hlo_computation.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/statusor.h"

namespace xla {

// Some lightweight utilities intended to make HLO instruction creation more
// ergonomic.  We don't have a complete set of helpers yet -- I expect we'll
// expand this interface as needed on an ad-hoc basis.

// Creates a binary HLO instruction and adds it to the computation containing
// `lhs` and `rhs` (`lhs` and `rhs` must be in the same computation).
StatusOr<HloInstruction*> MakeBinaryHlo(HloOpcode opcode, HloInstruction* lhs,
                                        HloInstruction* rhs);

// Creates a pad HLO instruction and adds it to the computation containing
// `operand` and `padding_value` (`operand` and `padding_value` must be in the
// same computation).
StatusOr<HloInstruction*> MakePadHlo(HloInstruction* operand,
                                     HloInstruction* padding_value,
                                     const PaddingConfig& padding_config);

// Creates a slice HLO instruction and adds it to the computation containing
// `operand`.
StatusOr<HloInstruction*> MakeSliceHlo(HloInstruction* operand,
                                       absl::Span<const int64> start_indices,
                                       absl::Span<const int64> limit_indices,
                                       absl::Span<const int64> strides);

// Creates a convolution HLO instruction and adds it to the computation
// containing `lhs` and `rhs` (`lhs` and `rhs` must be in the same computation).
StatusOr<HloInstruction*> MakeConvolveHlo(
    HloInstruction* lhs, HloInstruction* rhs, int64 feature_group_count,
    const Window& window, const ConvolutionDimensionNumbers& dimension_numbers,
    const PrecisionConfig& precision_config);

// Creates a transpose HLO instruction and adds it to the computation containing
// `operand`.
StatusOr<HloInstruction*> MakeTransposeHlo(HloInstruction* operand,
                                           absl::Span<const int64> dimensions);

// Creates a reshape HLO instruction and adds it to the computation containing
// `operand`.
StatusOr<HloInstruction*> MakeReshapeHlo(const Shape& result_shape,
                                         HloInstruction* operand);

StatusOr<HloInstruction*> MakeReshapeHlo(
    absl::Span<const int64> result_shape_dim_bounds, HloInstruction* operand);

// Creates a dynamic-slice HLO instruction and adds it to the computation
// containing `operand` and `start_indices` (`operand` and `start_indices` must
// be in the same computation).
StatusOr<HloInstruction*> MakeDynamicSliceHlo(
    HloInstruction* operand, HloInstruction* start_indices,
    absl::Span<const int64> slice_sizes);

// Creates a dynamic-update-slice HLO instruction and adds it to the computation
// containing `operand`, `update` and `start_indices` (`operand`, `update` and
// `start_indices` must be in the same computation).
StatusOr<HloInstruction*> MakeDynamicUpdateSliceHlo(
    HloInstruction* operand, HloInstruction* update,
    HloInstruction* start_indices);

// Creates a broadcast HLO instruction and adds it to the computation containing
// `operand`.
StatusOr<HloInstruction*> MakeBroadcastHlo(
    HloInstruction* operand, absl::Span<const int64> broadcast_dimensions,
    absl::Span<const int64> result_shape_bounds);

// Creates a GetTupleElement HLO instruction and adds it to the computation
// containing `operand`.
StatusOr<HloInstruction*> MakeGetTupleElementHlo(HloInstruction* operand,
                                                 int64 index);

// Creates a Concatenate HLO instruction and adds it to the computation
// containing `operands` (`operands` must be non-empty and every element must be
// contained in the same computation).
StatusOr<HloInstruction*> MakeConcatHlo(
    absl::Span<HloInstruction* const> operands, int64 dimension);

// Creates a Dot HLO instruction and adds it to the computation containing `lhs`
// and `rhs` (both must be in the same computation).
StatusOr<HloInstruction*> MakeDotHlo(HloInstruction* lhs, HloInstruction* rhs,
                                     const DotDimensionNumbers& dim_numbers,
                                     const PrecisionConfig& precision_config);

// Creates a Map HLO instruction and adds it to the computation containing the
// operands. All operands must be in the same computation.
StatusOr<HloInstruction*> MakeMapHlo(absl::Span<HloInstruction* const> operands,
                                     HloComputation* map_computation);

// Creates a Reduce HLO instruction and adds it to the computation containing
// the operand. This will create the sub-computation needed for the reduction in
// the given module. binary_opcode should represent a binary operation.
StatusOr<HloInstruction*> MakeReduceHlo(HloInstruction* operand,
                                        HloInstruction* init_value,
                                        HloOpcode binary_opcode,
                                        HloModule* module);

// Creates a Select HLO instruction and adds it to the computation containing
// the predicate. The on_true and on_false instructions must also be contained
// in the same computation.
StatusOr<HloInstruction*> MakeSelectHlo(HloInstruction* pred,
                                        HloInstruction* on_true,
                                        HloInstruction* on_false);

// Creates an R1 Constant HLO instruction of the given PrimitiveType with the
// given values and adds it to the given computation.
template <typename NativeT>
StatusOr<HloInstruction*> MakeR1ConstantHlo(HloComputation* computation,
                                            PrimitiveType type,
                                            absl::Span<const NativeT> values) {
  Literal literal = LiteralUtil::CreateR1<NativeT>(values);
  if (literal.shape().element_type() != type) {
    TF_ASSIGN_OR_RETURN(literal, literal.Convert(type));
  }
  return computation->AddInstruction(
      HloInstruction::CreateConstant(std::move(literal)));
}

// -----------------------------------------------------------------------------
// Some other miscellaneous helpers to generate common HLO patterns.  All of
// these add all the instructions they generate into the computation containing
// their operand(s).

// Collapses (via reshape) the first N (logical) dimensions of `operand` into a
// single leading dimension.  `operand` must have rank > `n` and `n` must not be
// 0.
//
// For instance if `operand` has shape f32[7,8,9] and n is 2 then the output is
// the `operand` reshaped to [56,9].
StatusOr<HloInstruction*> CollapseFirstNDims(HloInstruction* operand, int64 n);

// Prepends `n` degenerate dimensions (dimensions with bound = 1) to `operand`
// using a reshape.
//
// For instance if operand has shape f32[3,4,5] then this returns the operand
// reshaped to f32[1,3,4,5].  If the operand is a f32 scalar (i.e. has shape
// f32[]) then this returns the operand reshaped to f32[1].
StatusOr<HloInstruction*> PrependDegenerateDims(HloInstruction* operand,
                                                int64 n);

// Expands (via reshape) the first (logical) dimension of `operand` into a
// sequence of `expanded_dims` dimensions.  `operand` must at least be of rank 1
// and the number of elements in its first dimension must be equal to the
// product of `expanded_dims`.
//
// For instance if `operand` has shape f32[200,9,7] and expanded_dims is
// {2,5,20} the result is `operand` reshaped to [2,5,20,9,7].
StatusOr<HloInstruction*> ExpandFirstDimIntoNDims(
    HloInstruction* operand, absl::Span<const int64> expanded_dims);

// Elides (via reshape) a set of degenerate dimensions (dimensions containing
// exactly one element), `dims_to_elide` from `operand`.  Every dimension in
// `dims_to_elide` must be a degenerate dimension.  `dims_to_elide` must be
// sorted and not contain duplicates.
//
// For example if `operand` is of shape f32[19,1,20,1,7,1,9] and dims_to_elide
// is {1,5} then the result is `operand` reshaped to [19,20,1,7,9].
StatusOr<HloInstruction*> ElideDegenerateDims(
    HloInstruction* operand, absl::Span<const int64> dims_to_elide);

// Inserts (via reshape) a set of degenerate dimensions (dimensions containing
// exactly one element), `dims_to_insert` into `operand`. The dimensions in
// `dims_to_insert` refer to the dimensions in the result, and hence should be
// less than the rank of the result. Also, `dims_to_insert` must be sorted.
//
// For example, if `operand` is of shape f32[12,21,8,34] and dims_to_insert is
// {0, 2}, then the result is `operand` reshaped to [1,12,1,21,8,34].
StatusOr<HloInstruction*> InsertDegenerateDims(
    HloInstruction* operand, absl::Span<const int64> dims_to_insert);

// Pads `operand` (which must have rank 1) with `zeros_to_prepend` zeros in the
// front and `zeros_to_append` zeros in the back.
StatusOr<HloInstruction*> PadVectorWithZeros(HloInstruction* operand,
                                             int64 zeros_to_prepend,
                                             int64 zeros_to_append);

// Broadcasts a zero value of type `element_type` into a tensor with element
// type `element_type` and dimension bounds `broadcast_dimensions`.  The
// broadcast instruction is emitted into `computation`.
StatusOr<HloInstruction*> BroadcastZeros(
    HloComputation* computation, PrimitiveType element_type,
    absl::Span<const int64> broadcast_dimensions);

// Creates a HLO computation that takes arguments of type `domain` and produces
// a value of type `range`.
StatusOr<std::unique_ptr<HloComputation>> CreateComputationWithSignature(
    absl::Span<const Shape* const> domain, const Shape& range,
    absl::string_view name);

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_HLO_CREATION_UTILS_H_
