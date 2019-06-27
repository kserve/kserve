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

#include "tensorflow/compiler/xla/service/llvm_ir/dynamic_update_slice_util.h"
#include "tensorflow/compiler/xla/service/gpu/parallel_loop_emitter.h"
#include "tensorflow/compiler/xla/service/gpu/partition_assignment.h"
#include "tensorflow/compiler/xla/service/llvm_ir/fused_ir_emitter.h"
#include "tensorflow/compiler/xla/service/llvm_ir/llvm_util.h"
#include "tensorflow/compiler/xla/service/llvm_ir/loop_emitter.h"

namespace xla {
namespace llvm_ir {

bool CanUpdateDynamicSliceInPlace(HloInstruction* dynamic_update_slice,
                                  const BufferAssignment& assignment) {
  CHECK_EQ(HloOpcode::kDynamicUpdateSlice, dynamic_update_slice->opcode());
  const HloInstruction* operand = dynamic_update_slice->operand(0);
  return assignment.HasTopLevelAllocation(dynamic_update_slice) &&
         assignment.HasTopLevelAllocation(operand) &&
         assignment.SharesTopLevelSlice(dynamic_update_slice, operand);
}

// Shared implementation of EmitDynamicUpdateSliceInPlace and
// EmitFusedDynamicUpdateSliceInPlace.
//
// Emits a sequential loop if launch_dimensions is null.
static Status EmitDynamicUpdateSliceInPlaceImpl(
    const Shape& update_shape, const ElementGenerator& start_indices_generator,
    bool is_signed, ElementGenerator update_array_generator,
    const IrArray& output_array, const gpu::LaunchDimensions* launch_dimensions,
    absl::string_view name, llvm::IRBuilder<>* b) {
  const Shape& output_shape = output_array.GetShape();

  // Read start indices from start_indices_generator.
  const int64 rank = ShapeUtil::Rank(output_shape);
  IrArray::Index start_index(b->getInt64Ty(), rank);
  for (int64 i = 0; i < rank; ++i) {
    IrArray::Index dim_index({b->getInt64(i)});
    TF_ASSIGN_OR_RETURN(start_index[i], start_indices_generator(dim_index));
    llvm::Value* output_dim_size = llvm::ConstantInt::get(
        start_index[i]->getType(), output_shape.dimensions(i));
    llvm::Value* update_dim_size = llvm::ConstantInt::get(
        start_index[i]->getType(), update_shape.dimensions(i));

    // Clamp the start index so that the update region fits in the operand.
    // start_index = clamp(start_index, 0, output_dim_size - update_dim_size)
    llvm::Value* max_bound = b->CreateSub(output_dim_size, update_dim_size);
    llvm::Value* zero = llvm::ConstantInt::get(start_index[i]->getType(), 0);
    start_index[i] =
        b->CreateSelect(b->CreateICmp(is_signed ? llvm::ICmpInst::ICMP_SGE
                                                : llvm::ICmpInst::ICMP_UGE,
                                      zero, start_index[i]),
                        zero, start_index[i]);

    start_index[i] =
        b->CreateSelect(b->CreateICmp(is_signed ? llvm::ICmpInst::ICMP_SLE
                                                : llvm::ICmpInst::ICMP_ULE,
                                      max_bound, start_index[i]),
                        max_bound, start_index[i]);
  }

  auto loop_body_emitter = [&](const IrArray::Index& update_index) -> Status {
    // Calculate output_index, where we'll write the value from update.  For
    // each dimension,
    //
    //   output_index[dim] = start_index[dim] + update_index[dim]
    //
    IrArray::Index output_index(start_index.GetType(), rank);
    for (int64 i = 0; i < rank; ++i) {
      llvm::Value* start_index0 =
          b->CreateSExtOrBitCast(start_index[i], update_index[i]->getType());
      output_index[i] = b->CreateAdd(start_index0, update_index[i]);
    }

    // Do output[output_index] = update[update_index].
    TF_ASSIGN_OR_RETURN(llvm::Value * update_data,
                        update_array_generator(update_index));
    output_array.EmitWriteArrayElement(output_index, update_data, b);
    return Status::OK();
  };

  if (launch_dimensions != nullptr) {
    return gpu::ParallelLoopEmitter(loop_body_emitter, update_shape,
                                    *launch_dimensions, b)
        .EmitLoop(name);
  }
  return LoopEmitter(loop_body_emitter, update_shape, b).EmitLoop(name);
}

Status EmitDynamicUpdateSliceInPlace(absl::Span<const IrArray> operand_arrays,
                                     const IrArray& output_array,
                                     absl::string_view name,
                                     llvm::IRBuilder<>* b) {
  VLOG(2) << "EmitDynamicUpdateSliceInPlace for " << name;

  // No need to use operand_arrays[0], the input array of the
  // dynamic-update-slice, because we know it aliases the op's output.
  IrArray update_array = operand_arrays[1];
  IrArray start_indices_array = operand_arrays[2];
  Shape output_shape = output_array.GetShape();
  Shape update_shape = update_array.GetShape();

  ElementGenerator start_indices_generator = [&](const IrArray::Index& index) {
    return start_indices_array.EmitReadArrayElement(index, b);
  };
  ElementGenerator update_array_generator = [&](const IrArray::Index& index) {
    return update_array.EmitReadArrayElement(index, b);
  };

  bool is_signed = ShapeUtil::ElementIsSigned(start_indices_array.GetShape());
  return EmitDynamicUpdateSliceInPlaceImpl(
      update_shape, start_indices_generator, is_signed, update_array_generator,
      output_array, /*launch_dimensions=*/nullptr, name, b);
}

// Shared implementation for EmitFusedDynamicUpdateSliceInPlace and
// EmitParallelFusedDynamicUpdateSliceInPlace.
//
// Emits a sequential loop if launch_dimensions is null.
static Status EmitFusedDynamicUpdateSliceInPlaceImpl(
    HloInstruction* fusion,
    GeneratorForOperandIrArrays operand_arrays_generator,
    const IrArray& fusion_output_array, ElementalIrEmitter* elemental_emitter,
    const gpu::LaunchDimensions* launch_dimensions, llvm::IRBuilder<>* b) {
  CHECK_EQ(fusion->opcode(), HloOpcode::kFusion);
  VLOG(2) << "EmitFusedDynamicUpdateSliceInPlace for "
          << fusion->ToShortString();

  auto* dynamic_update_slice = fusion->fused_expression_root();

  const auto* update = dynamic_update_slice->operand(1);
  const auto* start_indices = dynamic_update_slice->operand(2);
  Shape update_shape = update->shape();

  // Our in-place dynamic-update-slice implementation emits a loop over
  // update_shape.  To emit a cache-friendly loop, we need to know that shape's
  // layout.
  //
  // update_shape is inside a fusion node -- it's never materialized in memory
  // and thus doesn't have a layout.  In this case we use the layout of the
  // fusion node for iteration, since that corresponds to the order in memory of
  // the buffer we'll be writing to.
  //
  // (This isn't necessarily optimal; in some cases it might be faster to peek
  // through the chain of ops that gives us the update operand and use the
  // layout of its source buffer(s).  But this is no worse than we do with
  // fusion elsewhere.)
  TF_RETURN_IF_ERROR(
      LayoutUtil::CopyLayoutBetweenShapes(fusion->shape(), &update_shape));

  // Create element generators for update and start_indices.
  FusedIrEmitter fused_emitter(std::move(operand_arrays_generator),
                               elemental_emitter);
  TF_RETURN_IF_ERROR(dynamic_update_slice->Accept(&fused_emitter));
  ElementGenerator update_array_generator = fused_emitter.GetGenerator(update);
  ElementGenerator start_indices_generator =
      fused_emitter.GetGenerator(start_indices);

  bool is_signed = ShapeUtil::ElementIsSigned(start_indices->shape());
  return EmitDynamicUpdateSliceInPlaceImpl(
      update_shape, start_indices_generator, is_signed, update_array_generator,
      fusion_output_array, launch_dimensions, IrName(fusion), b);
}

Status EmitFusedDynamicUpdateSliceInPlace(
    HloInstruction* fusion,
    GeneratorForOperandIrArrays operand_arrays_generator,
    const IrArray& fusion_output_array, ElementalIrEmitter* elemental_emitter,
    llvm::IRBuilder<>* b) {
  return EmitFusedDynamicUpdateSliceInPlaceImpl(
      fusion, std::move(operand_arrays_generator), fusion_output_array,
      elemental_emitter,
      /*launch_dimensions=*/nullptr, b);
}

Status EmitParallelFusedDynamicUpdateSliceInPlace(
    HloInstruction* fusion,
    GeneratorForOperandIrArrays operand_arrays_generator,
    const IrArray& fusion_output_array, ElementalIrEmitter* elemental_emitter,
    const gpu::LaunchDimensions& launch_dimensions, llvm::IRBuilder<>* b) {
  return EmitFusedDynamicUpdateSliceInPlaceImpl(
      fusion, std::move(operand_arrays_generator), fusion_output_array,
      elemental_emitter, &launch_dimensions, b);
}

}  // namespace llvm_ir
}  // namespace xla
