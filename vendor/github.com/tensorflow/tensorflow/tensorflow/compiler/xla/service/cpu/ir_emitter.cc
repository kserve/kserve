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

#include "tensorflow/compiler/xla/service/cpu/ir_emitter.h"

#include <stddef.h>
#include <stdint.h>
#include <algorithm>
#include <iterator>
#include <limits>
#include <memory>
#include <utility>
#include <vector>

#include "absl/container/flat_hash_map.h"
#include "absl/container/flat_hash_set.h"
#include "tensorflow/core/lib/math/math_util.h"
#include "tensorflow/core/platform/logging.h"
// IWYU pragma: no_include "llvm/IR/Intrinsics.gen.inc"
#include "absl/strings/str_cat.h"
#include "absl/strings/str_format.h"
#include "absl/types/span.h"
#include "llvm/CodeGen/TargetRegisterInfo.h"
#include "llvm/CodeGen/TargetSubtargetInfo.h"
#include "llvm/IR/BasicBlock.h"
#include "llvm/IR/Constants.h"
#include "llvm/IR/GlobalVariable.h"
#include "llvm/IR/Instructions.h"
#include "llvm/IR/Intrinsics.h"
#include "llvm/IR/LLVMContext.h"
#include "tensorflow/compiler/xla/layout_util.h"
#include "tensorflow/compiler/xla/map_util.h"
#include "tensorflow/compiler/xla/service/buffer_assignment.h"
#include "tensorflow/compiler/xla/service/cpu/cpu_options.h"
#include "tensorflow/compiler/xla/service/cpu/cpu_runtime.h"
#include "tensorflow/compiler/xla/service/cpu/dot_op_emitter.h"
#include "tensorflow/compiler/xla/service/cpu/elemental_ir_emitter.h"
#include "tensorflow/compiler/xla/service/cpu/ir_emission_utils.h"
#include "tensorflow/compiler/xla/service/cpu/ir_function.h"
#include "tensorflow/compiler/xla/service/cpu/parallel_loop_emitter.h"
#include "tensorflow/compiler/xla/service/cpu/shape_partition.h"
#include "tensorflow/compiler/xla/service/cpu/simple_orc_jit.h"
#include "tensorflow/compiler/xla/service/elemental_ir_emitter.h"
#include "tensorflow/compiler/xla/service/hlo_casting_utils.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_instructions.h"
#include "tensorflow/compiler/xla/service/hlo_opcode.h"
#include "tensorflow/compiler/xla/service/llvm_ir/buffer_assignment_util.h"
#include "tensorflow/compiler/xla/service/llvm_ir/dynamic_update_slice_util.h"
#include "tensorflow/compiler/xla/service/llvm_ir/fused_ir_emitter.h"
#include "tensorflow/compiler/xla/service/llvm_ir/llvm_loop.h"
#include "tensorflow/compiler/xla/service/llvm_ir/llvm_util.h"
#include "tensorflow/compiler/xla/service/llvm_ir/tuple_ops.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/status_macros.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/compiler/xla/window_util.h"
#include "tensorflow/core/lib/core/bits.h"
#include "tensorflow/core/lib/core/errors.h"

namespace xla {

namespace {
using llvm_ir::AsStringRef;
using llvm_ir::IrName;
using llvm_ir::SetToFirstInsertPoint;
namespace gtl = tensorflow::gtl;
}  // namespace

namespace cpu {

IrEmitter::IrEmitter(
    const HloModule& hlo_module, const BufferAssignment& assignment,
    llvm::Module* llvm_module,
    std::unordered_map<const HloInstruction*, int64> instruction_to_profile_idx,
    std::unordered_map<const HloComputation*, int64> computation_to_profile_idx,
    const TargetMachineFeatures* target_machine_features)
    : assignment_(assignment),
      module_(llvm_module),
      arch_type_(llvm::Triple(llvm_module->getTargetTriple()).getArch()),
      b_(llvm_module->getContext()),
      instruction_to_profile_idx_(std::move(instruction_to_profile_idx)),
      computation_to_profile_idx_(std::move(computation_to_profile_idx)),
      alias_analysis_(hlo_module, assignment, &llvm_module->getContext()),
      hlo_module_config_(hlo_module.config()),
      is_top_level_computation_(false),
      target_machine_features_(*target_machine_features) {
  b_.setFastMathFlags(llvm_ir::GetFastMathFlags(
      /*fast_math_enabled=*/hlo_module_config_.debug_options()
          .xla_cpu_enable_fast_math()));
  Status s = GatherComputationsByAllocationType(
      &hlo_module, &thread_local_computations_, &global_computations_);
  absl::c_sort(thread_local_computations_);
  absl::c_sort(global_computations_);
  TF_CHECK_OK(s) << "Should have failed buffer assignment.";
}

StatusOr<llvm::Function*> IrEmitter::EmitComputation(
    HloComputation* computation, const string& function_name_prefix,
    bool is_top_level_computation,
    absl::Span<HloInstruction* const> instruction_order) {
  string function_name = name_uniquer_.GetUniqueName(function_name_prefix);
  VLOG(2) << "Emitting IR for CPU function [" << function_name_prefix << "]";
  is_top_level_computation_ = is_top_level_computation;
  num_dynamic_loop_bounds_ = 0;
  if (!computation->root_instruction()->outer_dimension_partitions().empty()) {
    num_dynamic_loop_bounds_ =
        computation->root_instruction()->outer_dimension_partitions().size();
  }

  if (computation->root_instruction()->opcode() != HloOpcode::kOutfeed) {
    TF_ASSIGN_OR_RETURN(
        computation_root_allocation_,
        assignment_.GetUniqueTopLevelSlice(computation->root_instruction()));
  }

  for (const HloInstruction* param : computation->parameter_instructions()) {
    TF_ASSIGN_OR_RETURN(BufferAllocation::Slice param_slice,
                        assignment_.GetUniqueTopLevelSlice(param));
    computation_parameter_allocations_[param_slice.allocation()->index()] =
        param->parameter_number();
  }

  InitializeIrFunction(function_name);
  // The rdtscp instruction is x86 specific.  We will fallback to LLVM's generic
  // readcyclecounter if it is unavailable.
  bool use_rdtscp = arch_type_ == llvm::Triple::ArchType::x86 ||
                    arch_type_ == llvm::Triple::ArchType::x86_64;
  profiling_state_ = ProfilingState(use_rdtscp);
  TF_RETURN_IF_ERROR(computation->AcceptOrdered(this, instruction_order));
  llvm::Function* ir_function = compute_function_->function();
  InsertOrDie(&emitted_functions_, computation, ir_function);
  // Delete 'compute_function', finalizing 'ir_function' and restoring caller
  // IR insert point.
  compute_function_.reset();
  computation_root_allocation_ = BufferAllocation::Slice();
  computation_parameter_allocations_.clear();
  return ir_function;
}

void IrEmitter::InitializeIrFunction(const string& function_name) {
  // Functions with local linkage get an inlining bonus.  Because we know
  // a-priori that embedded functions (non-entry functions) will not have its
  // name resolved, give it local linkage.
  llvm::Function::LinkageTypes linkage =
      is_top_level_computation_ ? llvm::GlobalValue::ExternalLinkage
                                : llvm::GlobalValue::InternalLinkage;
  // Create and initialize new IrFunction.
  compute_function_.reset(new IrFunction(
      function_name, linkage,
      options::OptimizeForSizeRequested(hlo_module_config_),
      hlo_module_config_.debug_options().xla_cpu_enable_fast_math(), module_,
      &b_, num_dynamic_loop_bounds_));
}

IrEmitter::~IrEmitter() {}

Status IrEmitter::HandleBitcast(HloInstruction* bitcast) {
  VLOG(2) << "HandleBitcast: " << bitcast->ToString();
  emitted_value_[bitcast] =
      BitCast(GetEmittedValueFor(bitcast->operand(0)),
              IrShapeType(bitcast->shape())->getPointerTo(),
              AsStringRef(IrName(bitcast)));
  return Status::OK();
}

llvm::Constant* IrEmitter::EmitGlobalForLiteral(const Literal& literal) {
  llvm::Constant* initializer =
      llvm_ir::ConvertLiteralToIrConstant(literal, module_);
  llvm::GlobalVariable* result_global = new llvm::GlobalVariable(
      /*Module=*/*module_,
      /*Type=*/initializer->getType(),
      /*isConstant=*/true,
      /*Linkage=*/llvm::GlobalValue::PrivateLinkage,
      /*Initializer=*/initializer,
      /*Name=*/"");
  result_global->setAlignment(MinimumAlignmentForShape(literal.shape()));
  return llvm::ConstantExpr::getBitCast(
      result_global, IrShapeType(literal.shape())->getPointerTo());
}

Status IrEmitter::EmitConstantGlobals() {
  for (const BufferAllocation& allocation : assignment_.Allocations()) {
    if (!allocation.is_constant()) {
      continue;
    }

    const Literal& literal = llvm_ir::LiteralForConstantAllocation(allocation);
    llvm::Constant* global_for_const;
    auto it = emitted_literals_.find(&literal);
    if (it != emitted_literals_.end()) {
      global_for_const = it->second;
    } else {
      global_for_const = EmitGlobalForLiteral(literal);
      InsertOrDie(&emitted_literals_, &literal, global_for_const);
    }

    InsertOrDie(&constant_buffer_to_global_, allocation.index(),
                global_for_const);
  }

  return Status::OK();
}

Status IrEmitter::HandleConstant(HloInstruction* constant) {
  VLOG(2) << "HandleConstant: " << constant->ToString();
  // IrEmitter::EmitConstantGlobals has already taken care of emitting the body
  // of the constant.
  return EmitTargetAddressForOp(constant);
}

Status IrEmitter::HandleCopy(HloInstruction* copy) {
  if (ShapeUtil::IsTuple(copy->shape())) {
    // kCopy shallow copies a tuple so just memcpy the top-level buffer.
    TF_RETURN_IF_ERROR(EmitTargetAddressForOp(copy));
    return EmitMemcpy(*(copy->operand(0)), *copy);
  } else if (ShapeUtil::IsArray(copy->shape())) {
    // Use the elemental emitter for array shapes.
    return DefaultAction(copy);
  }
  return Unimplemented("unsupported operand type %s for copy instruction",
                       PrimitiveType_Name(copy->shape().element_type()));
}

// Calculate the alignment of a buffer allocated for a given primitive type.
int IrEmitter::MinimumAlignmentForPrimitiveType(PrimitiveType primitive_type) {
  int64 byte_size = ShapeUtil::ByteSizeOfPrimitiveType(primitive_type);
  DCHECK_GE(byte_size, 0);
  // Largest scalar is a complex64 so we don't need to worry about the
  // int64->int truncation here.
  DCHECK_LE(byte_size, 8);
  return byte_size;
}

int64 IrEmitter::ByteSizeOf(const Shape& shape) const {
  return llvm_ir::ByteSizeOf(shape, module_->getDataLayout());
}

// Calculate the alignment of a buffer allocated for a given shape.
int IrEmitter::MinimumAlignmentForShape(const Shape& shape) {
  if (ShapeUtil::IsScalar(shape)) {
    return MinimumAlignmentForPrimitiveType(shape.element_type());
  }

  int64 buffer_size = ByteSizeOf(shape);
  DCHECK_GE(buffer_size, 0);
  DCHECK_LE(buffer_size, SIZE_MAX);

  return target_machine_features_.minimum_alignment_for_allocation(buffer_size);
}

void IrEmitter::AttachAlignmentMetadataForLoad(llvm::LoadInst* load,
                                               const Shape& shape) {
  int alignment = MinimumAlignmentForShape(shape);
  if (alignment > 1) {
    llvm_ir::SetAlignmentMetadataForLoad(load, alignment);
  }
}

void IrEmitter::AttachAlignmentMetadataForLoad(llvm::LoadInst* load,
                                               int64 buffer_size) {
  int alignment =
      target_machine_features_.minimum_alignment_for_allocation(buffer_size);
  if (alignment > 1) {
    llvm_ir::SetAlignmentMetadataForLoad(load, alignment);
  }
}

void IrEmitter::AttachDereferenceableMetadataForLoad(llvm::LoadInst* load,
                                                     const Shape& shape) {
  AttachDereferenceableMetadataForLoad(load, ByteSizeOf(shape));
}

void IrEmitter::AttachDereferenceableMetadataForLoad(llvm::LoadInst* load,
                                                     int64 buffer_size) {
  if (buffer_size > 0) {
    llvm_ir::SetDereferenceableMetadataForLoad(load, buffer_size);
  }
}

Status IrEmitter::HandleGetTupleElement(HloInstruction* get_tuple_element) {
  // A tuple is an array of pointers, one for each operand. Each pointer points
  // to the output buffer of its corresponding operand. A GetTupleElement
  // instruction forwards a pointer to the tuple element buffer at the given
  // index.
  auto operand = get_tuple_element->operand(0);
  const Shape& shape = get_tuple_element->shape();
  emitted_value_[get_tuple_element] = llvm_ir::EmitGetTupleElement(
      shape, get_tuple_element->tuple_index(), MinimumAlignmentForShape(shape),
      GetEmittedValueFor(operand), &b_, module_);
  return Status::OK();
}

Status IrEmitter::HandleSelect(HloInstruction* select) {
  auto pred = select->operand(0);
  TF_RET_CHECK(pred->shape().element_type() == PRED);
  return DefaultAction(select);
}

Status IrEmitter::HandleTupleSelect(HloInstruction* tuple_select) {
  auto pred = tuple_select->operand(0);
  auto on_true = tuple_select->operand(1);
  auto on_false = tuple_select->operand(2);
  TF_RET_CHECK(pred->shape().element_type() == PRED);
  TF_RET_CHECK(ShapeUtil::IsScalar(pred->shape()));
  TF_RET_CHECK(ShapeUtil::IsTuple(tuple_select->shape()));
  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(tuple_select));
  llvm_ir::EmitTupleSelect(GetIrArrayFor(tuple_select), GetIrArrayFor(pred),
                           GetEmittedValueFor(on_true),
                           GetEmittedValueFor(on_false), &b_, module_);
  return Status::OK();
}

Status IrEmitter::HandleInfeed(HloInstruction* instruction) {
  HloInfeedInstruction* infeed = Cast<HloInfeedInstruction>(instruction);
  VLOG(2) << "HandleInfeed: " << infeed->ToString();

  // The infeed operation produces a two-element tuple containing data and a
  // token value. HloInfeedInstruction::infeed_shape gives us the data shape.
  const Shape& data_shape = infeed->infeed_shape();
  DCHECK(ShapeUtil::Equal(data_shape,
                          ShapeUtil::GetTupleElementShape(infeed->shape(), 0)));
  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(infeed));

  // Write the tuple index table.
  TF_ASSIGN_OR_RETURN(BufferAllocation::Slice data_slice,
                      assignment_.GetUniqueSlice(infeed, {0}));
  llvm::Value* data_address = EmitBufferPointer(data_slice, data_shape);
  TF_ASSIGN_OR_RETURN(BufferAllocation::Slice token_slice,
                      assignment_.GetUniqueSlice(infeed, {1}));
  llvm::Value* token_address = EmitBufferPointer(
      token_slice, ShapeUtil::GetTupleElementShape(infeed->shape(), 1));
  llvm_ir::EmitTuple(GetIrArrayFor(infeed), {data_address, token_address}, &b_,
                     module_);

  if (ShapeUtil::IsTuple(data_shape)) {
    TF_RET_CHECK(!ShapeUtil::IsNestedTuple(data_shape));

    // For a tuple, we first copy each of the internal elements to
    // their corresponding target locations. We then construct the
    // tuple outer buffer containing pointers to the internal
    // elements.
    std::vector<llvm::Value*> tuple_element_addresses;
    for (int64 i = 0; i < data_shape.tuple_shapes_size(); ++i) {
      TF_ASSIGN_OR_RETURN(BufferAllocation::Slice buffer,
                          assignment_.GetUniqueSlice(infeed, {0, i}));

      const Shape& tuple_element_shape =
          ShapeUtil::GetTupleElementShape(data_shape, i);

      // Only the outer tuple buffer's target address is obtained from
      // GetEmittedValueFor, to handle the case when Infeed is the root
      // instruction. Target addresses for internal elements can be obtained
      // from EmitBufferPointer.
      llvm::Value* tuple_element_address =
          EmitBufferPointer(buffer, tuple_element_shape);

      TF_RETURN_IF_ERROR(EmitXfeedTransfer(
          XfeedKind::kInfeed, tuple_element_shape, tuple_element_address));

      tuple_element_addresses.push_back(tuple_element_address);
    }

    llvm_ir::EmitTuple(llvm_ir::IrArray(data_address, data_shape),
                       tuple_element_addresses, &b_, module_);
  } else {
    TF_RETURN_IF_ERROR(
        EmitXfeedTransfer(XfeedKind::kInfeed, data_shape, data_address));
  }

  return Status::OK();
}

Status IrEmitter::EmitXfeedTransfer(XfeedKind kind, const Shape& shape,
                                    llvm::Value* program_buffer_address) {
  int64 length = ByteSizeOf(shape);
  if (length <= 0 || length > std::numeric_limits<int32>::max()) {
    return InvalidArgument(
        "xfeed (infeed or outfeed) buffer length %d is outside the valid "
        "size range",
        length);
  }
  int32 length_32 = static_cast<int32>(length);

  int32 shape_length;
  TF_ASSIGN_OR_RETURN(
      llvm::Value * shape_ptr,
      llvm_ir::EncodeSelfDescribingShapeConstant(shape, &shape_length, &b_));

  llvm::Type* int32_type = b_.getInt32Ty();
  llvm::Type* i8_ptr_type = llvm::Type::getInt8PtrTy(module_->getContext());
  llvm::FunctionType* acquire_type = llvm::FunctionType::get(
      i8_ptr_type,
      {/*run_options*/ i8_ptr_type, /*buffer_length*/ int32_type,
       /*shape_ptr*/ i8_ptr_type, /*shape_length*/ int32_type},
      /*isVarArg=*/false);

  llvm::Function* acquire_func;
  if (kind == XfeedKind::kInfeed) {
    acquire_func = llvm::cast<llvm::Function>(module_->getOrInsertFunction(
        runtime::kAcquireInfeedBufferForDequeueSymbolName, acquire_type));
  } else {
    acquire_func = llvm::cast<llvm::Function>(module_->getOrInsertFunction(
        runtime::kAcquireOutfeedBufferForPopulationSymbolName, acquire_type));
  }
  acquire_func->setCallingConv(llvm::CallingConv::C);

  llvm::FunctionType* release_type = llvm::FunctionType::get(
      b_.getVoidTy(),
      {/*run_options*/ i8_ptr_type, /*buffer_length*/ int32_type,
       /*buffer_ptr*/ i8_ptr_type, /*shape_ptr*/ i8_ptr_type,
       /*shape_length*/ int32_type},
      /*isVarArg=*/false);

  llvm::Function* release_func;
  if (kind == XfeedKind::kInfeed) {
    release_func = llvm::cast<llvm::Function>(module_->getOrInsertFunction(
        runtime::kReleaseInfeedBufferAfterDequeueSymbolName, release_type));
  } else {
    release_func = llvm::cast<llvm::Function>(module_->getOrInsertFunction(
        runtime::kReleaseOutfeedBufferAfterPopulationSymbolName, release_type));
  }
  release_func->setCallingConv(llvm::CallingConv::C);

  // Implementation note: this call informs the runtime that it wants a buffer
  // of size exactly 'length_32', and the runtime is responsible for
  // check-failing the process if there is a mismatch, versus passing us back a
  // buffer that we might overrun.
  llvm::Value* acquired_pointer = Call(
      acquire_func, {GetExecutableRunOptionsArgument(), b_.getInt32(length_32),
                     shape_ptr, b_.getInt32(shape_length)});

  if (kind == XfeedKind::kInfeed) {
    // Copy to the program buffer address from the acquired buffer.
    MemCpy(program_buffer_address, /*DstAlign=*/1, acquired_pointer,
           /*SrcAlign=*/1, length_32);
  } else {
    // Outfeed -- copy from the in-program address to the acquired buffer.
    MemCpy(acquired_pointer, /*DstAlign=*/1, program_buffer_address,
           /*SrcAlign=*/1, length_32);
  }

  Call(release_func, {GetExecutableRunOptionsArgument(), b_.getInt32(length_32),
                      acquired_pointer, shape_ptr, b_.getInt32(shape_length)});

  return Status::OK();
}

Status IrEmitter::HandleOutfeed(HloInstruction* outfeed) {
  // Outfeed produces no useful result, but it does return a token[] that can be
  // threaded through to other side effecting operations to ensure ordering.  In
  // the IR emitter we treat this token as a normal u8[] and thus need to insert
  // an entry for it in emitted_value_.
  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(outfeed));

  HloInstruction* operand = outfeed->operands()[0];
  const Shape& operand_shape = operand->shape();

  llvm::Value* value = GetEmittedValueFor(operand);
  if (!ShapeUtil::IsTuple(operand_shape)) {
    return EmitXfeedTransfer(XfeedKind::kOutfeed, operand_shape, value);
  }

  TF_RET_CHECK(!ShapeUtil::IsNestedTuple(operand_shape));

  for (int64 i = 0; i < operand_shape.tuple_shapes_size(); ++i) {
    const Shape& tuple_element_shape =
        ShapeUtil::GetTupleElementShape(operand_shape, i);
    llvm::Value* tuple_element = llvm_ir::EmitGetTupleElement(
        tuple_element_shape, i, MinimumAlignmentForShape(tuple_element_shape),
        value, &b_, module_);
    TF_RETURN_IF_ERROR(EmitXfeedTransfer(XfeedKind::kOutfeed,
                                         tuple_element_shape, tuple_element));
  }

  return Status::OK();
}

Status IrEmitter::HandleSort(HloInstruction* hlo) {
  const HloSortInstruction* sort = Cast<HloSortInstruction>(hlo);
  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(sort));
  Shape keys_shape = sort->keys()->shape();
  std::vector<llvm::Value*> destination_addresses(sort->operand_count());
  for (int64 i = 0; i < sort->operand_count(); ++i) {
    ShapeIndex shape_index =
        sort->values_count() > 0 ? ShapeIndex({i}) : ShapeIndex({});
    const HloInstruction* operand = sort->operand(i);
    // We assume that the layout of all involved operands and outputs is the
    // same.
    TF_RET_CHECK(
        LayoutUtil::LayoutsInShapesEqual(keys_shape, operand->shape()));
    TF_RET_CHECK(LayoutUtil::LayoutsInShapesEqual(
        keys_shape, ShapeUtil::GetSubshape(sort->shape(), shape_index)));

    // The sort is implemented in-place, therefore we first copy the operand
    // buffer to the output buffer if they are not the same.
    auto destination_buffer = GetAllocationSlice(*sort, shape_index);
    destination_addresses[i] =
        EmitBufferPointer(destination_buffer, operand->shape());
    auto source_address = GetAllocationSlice(*operand);
    if (destination_buffer != source_address) {
      int64 primitive_type_size =
          ShapeUtil::ByteSizeOfPrimitiveType(operand->shape().element_type());
      auto source_buffer = GetEmittedValueFor(operand);
      int64 size = ByteSizeOf(operand->shape());
      MemCpy(destination_addresses[i], /*DstAlign=*/primitive_type_size,
             source_buffer,
             /*SrcAlign=*/primitive_type_size, size);
    }
  }

  // Normalize the shape and the dimension to sort.
  Shape normalized_keys_shape =
      ShapeUtil::MakeShapeWithDescendingLayoutAndSamePhysicalLayout(keys_shape);
  int64 physical_dimension_to_sort = LayoutUtil::MakeLogicalToPhysical(
      keys_shape.layout())[sort->sort_dimension()];

  int64 sort_dimension_elements =
      normalized_keys_shape.dimensions(physical_dimension_to_sort);
  int64 higher_dimensions = 1;
  for (int64 i = 0; i < physical_dimension_to_sort; ++i) {
    higher_dimensions *= normalized_keys_shape.dimensions(i);
  }
  int64 lower_dimensions = 1;
  for (int64 i = ShapeUtil::Rank(normalized_keys_shape) - 1;
       i > physical_dimension_to_sort; --i) {
    lower_dimensions *= normalized_keys_shape.dimensions(i);
  }

  PrimitiveType keys_type = keys_shape.element_type();
  const char* fn_name = nullptr;
  llvm::Type* keys_native_type = nullptr;
  switch (keys_type) {
    case PRED:
      fn_name = runtime::kKeyValueSortPREDSymbolName;
      keys_native_type = b_.getInt8PtrTy();
      break;
    case S8:
      fn_name = runtime::kKeyValueSortS8SymbolName;
      keys_native_type = b_.getInt8PtrTy();
      break;
    case U8:
      fn_name = runtime::kKeyValueSortU8SymbolName;
      keys_native_type = b_.getInt8PtrTy();
      break;
    case S16:
      fn_name = runtime::kKeyValueSortS16SymbolName;
      keys_native_type = b_.getInt16Ty()->getPointerTo();
      break;
    case U16:
      fn_name = runtime::kKeyValueSortU16SymbolName;
      keys_native_type = b_.getInt16Ty()->getPointerTo();
      break;
    case F16:
      fn_name = runtime::kKeyValueSortF16SymbolName;
      keys_native_type = b_.getHalfTy()->getPointerTo();
      break;
    case S32:
      fn_name = runtime::kKeyValueSortS32SymbolName;
      keys_native_type = b_.getInt32Ty()->getPointerTo();
      break;
    case U32:
      fn_name = runtime::kKeyValueSortU32SymbolName;
      keys_native_type = b_.getInt32Ty()->getPointerTo();
      break;
    case F32:
      fn_name = runtime::kKeyValueSortF32SymbolName;
      keys_native_type = b_.getFloatTy()->getPointerTo();
      break;
    case S64:
      fn_name = runtime::kKeyValueSortS64SymbolName;
      keys_native_type = b_.getInt64Ty()->getPointerTo();
      break;
    case U64:
      fn_name = runtime::kKeyValueSortU64SymbolName;
      keys_native_type = b_.getInt64Ty()->getPointerTo();
      break;
    case F64:
      fn_name = runtime::kKeyValueSortF64SymbolName;
      keys_native_type = b_.getDoubleTy()->getPointerTo();
      break;
    default:
      return Unimplemented(
          "Element type %s not supported in the Sort op on CPU.",
          PrimitiveType_Name(keys_type));
  }

  llvm::FunctionType* key_value_sort_type = llvm::FunctionType::get(
      b_.getVoidTy(),
      {keys_native_type, b_.getInt64Ty(), b_.getInt64Ty(), b_.getInt64Ty(),
       b_.getInt8PtrTy()->getPointerTo(), b_.getInt32Ty(),
       b_.getInt32Ty()->getPointerTo()},
      /*isVarArg=*/false);
  auto* key_value_sort_func = llvm::cast<llvm::Function>(
      module_->getOrInsertFunction(fn_name, key_value_sort_type));
  key_value_sort_func->setCallingConv(llvm::CallingConv::C);
  key_value_sort_func->setDoesNotThrow();
  llvm::Value* values;
  llvm::Value* sizes;
  if (sort->values_count() == 0) {
    values = llvm::Constant::getNullValue(b_.getInt8PtrTy()->getPointerTo());
    sizes = llvm::Constant::getNullValue(b_.getInt32Ty()->getPointerTo());
  } else {
    values = llvm_ir::EmitAllocaAtFunctionEntryWithCount(
        b_.getInt8PtrTy(), b_.getInt32(sort->values_count()),
        "cc_values_alloca", &b_);
    sizes = llvm_ir::EmitAllocaAtFunctionEntryWithCount(
        b_.getInt32Ty(), b_.getInt32(sort->values_count()), "cc_sizes_alloca",
        &b_);
    for (int64 i = 0; i < sort->values_count(); ++i) {
      llvm::Value* value_as_i8ptr =
          PointerCast(destination_addresses[i + 1], b_.getInt8PtrTy());
      llvm::Value* slot_in_values_alloca =
          ConstInBoundsGEP1_32(b_.getInt8PtrTy(), values, i);
      Store(value_as_i8ptr, slot_in_values_alloca);
      llvm::Value* slot_in_sizes_alloca =
          ConstInBoundsGEP1_32(b_.getInt32Ty(), sizes, i);
      llvm::Value* size = b_.getInt32(ShapeUtil::ByteSizeOfPrimitiveType(
          sort->operand(i + 1)->shape().element_type()));
      Store(size, slot_in_sizes_alloca);
    }
  }

  Call(key_value_sort_func,
       {PointerCast(destination_addresses[0], keys_native_type),
        b_.getInt64(higher_dimensions), b_.getInt64(sort_dimension_elements),
        b_.getInt64(lower_dimensions), values,
        b_.getInt32(sort->values_count()), sizes});

  if (sort->values_count() > 0) {
    llvm_ir::EmitTuple(GetIrArrayFor(sort), destination_addresses, &b_,
                       module_);
  }
  return Status::OK();
}

Status IrEmitter::HandleTuple(HloInstruction* tuple) {
  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(tuple));
  std::vector<llvm::Value*> base_ptrs;
  for (auto operand : tuple->operands()) {
    base_ptrs.push_back(GetEmittedValueFor(operand));
  }
  llvm_ir::EmitTuple(GetIrArrayFor(tuple), base_ptrs, &b_, module_);
  return Status::OK();
}

llvm::Value* IrEmitter::EmitElementalMap(
    const HloMapInstruction& map_instr,
    absl::Span<llvm::Value* const> elemental_operands, absl::string_view name) {
  return EmitThreadLocalCall(*map_instr.to_apply(), elemental_operands, name);
}

StatusOr<llvm::Value*> IrEmitter::EmitTargetElementLoopBodyForReduceWindow(
    HloReduceWindowInstruction* reduce_window,
    const llvm_ir::IrArray::Index& index) {
  const HloInstruction* operand = reduce_window->operand(0);
  const Window& window = reduce_window->window();

  // We fold inputs into the accumulator and initialize it to
  // the initial value on the reduce_window.
  PrimitiveType operand_element_type = operand->shape().element_type();
  llvm::Value* accumulator_address = llvm_ir::EmitAllocaAtFunctionEntry(
      llvm_ir::PrimitiveTypeToIrType(operand_element_type, module_),
      "reduce_window_accumulator_address", &b_,
      MinimumAlignmentForPrimitiveType(operand_element_type));
  Store(Load(GetEmittedValueFor(reduce_window->operand(1))),
        accumulator_address);

  llvm_ir::ForLoopNest loops(IrName(reduce_window, "inner"), &b_);
  std::vector<int64> window_size;
  for (const auto& dim : window.dimensions()) {
    window_size.push_back(dim.size());
  }
  const llvm_ir::IrArray::Index window_index = loops.AddLoopsForShape(
      ShapeUtil::MakeShape(operand_element_type, window_size), "window");
  CHECK_EQ(window_index.size(), index.size());

  SetToFirstInsertPoint(loops.GetInnerLoopBodyBasicBlock(), &b_);

  llvm_ir::IrArray::Index input_index(b_.getInt64Ty(), index.size());
  llvm::Value* in_bounds_condition = nullptr;
  for (size_t i = 0; i < index.size(); ++i) {
    llvm::Value* strided_index =
        NSWMul(index[i], b_.getInt64(window.dimensions(i).stride()));
    input_index[i] = NSWSub(
        NSWAdd(strided_index,
               NSWMul(window_index[i],
                      b_.getInt64(window.dimensions(i).window_dilation()))),
        b_.getInt64(window.dimensions(i).padding_low()));

    // We need to verify that we are not in the dilated base area.
    llvm::Value* dilation_condition = ICmpEQ(
        SRem(input_index[i], b_.getInt64(window.dimensions(i).base_dilation())),
        b_.getInt64(0));
    if (in_bounds_condition == nullptr) {
      in_bounds_condition = dilation_condition;
    } else {
      in_bounds_condition = And(in_bounds_condition, dilation_condition);
    }

    // Apply base dilation to the index.
    input_index[i] =
        SDiv(input_index[i], b_.getInt64(window.dimensions(i).base_dilation()));

    // We need to check if 0 <= input_index[i] < bound, as otherwise we are in
    // the padding so that we can skip the computation. That is equivalent to
    // input_index[i] < bound as an *unsigned* comparison, since a negative
    // value will wrap to a large positive value.
    llvm::Value* index_condition =
        ICmpULT(input_index[i],
                b_.getInt64(ShapeUtil::GetDimension(operand->shape(), i)));
    if (in_bounds_condition == nullptr) {
      in_bounds_condition = index_condition;
    } else {
      in_bounds_condition = And(in_bounds_condition, index_condition);
    }
  }
  CHECK(in_bounds_condition != nullptr);

  llvm_ir::LlvmIfData if_data =
      llvm_ir::EmitIfThenElse(in_bounds_condition, "in-bounds", &b_);
  SetToFirstInsertPoint(if_data.true_block, &b_);

  // We are not in the padding, so carry out the computation.
  llvm_ir::IrArray input_array(GetIrArrayFor(operand));
  llvm::Value* input_value = input_array.EmitReadArrayElement(input_index, &b_);
  llvm::Value* result = EmitThreadLocalCall(
      *reduce_window->to_apply(), {Load(accumulator_address), input_value},
      "reducer_function");
  Store(result, accumulator_address);

  SetToFirstInsertPoint(loops.GetOuterLoopExitBasicBlock(), &b_);
  return Load(accumulator_address);
}

Status IrEmitter::HandleReduceWindow(HloInstruction* reduce_window) {
  TF_RETURN_IF_ERROR(ElementTypesSameAndSupported(
      /*instruction=*/*reduce_window,
      /*operands=*/{reduce_window->operand(0)},
      /*supported_types=*/{F32, BF16, S32, F16}));

  // Pseudo code for reduce window:
  //
  //   for (coordinates O in the output)
  //     value = init_value;
  //     for (coordinates W in the window)
  //       for each index i:
  //         input coordinates I_i = O_i * stride_i + W_i - pad_low_i
  //       if I within bounds of input:
  //         value = function(value, input(I));
  //     output(O) = value;
  //
  // This is completely un-optimized and just here to have something
  // that works.
  return EmitTargetElementLoop(
      reduce_window, [&](const llvm_ir::IrArray::Index& index) {
        return EmitTargetElementLoopBodyForReduceWindow(
            Cast<HloReduceWindowInstruction>(reduce_window), index);
      });
}

Status IrEmitter::HandleSelectAndScatter(HloInstruction* select_and_scatter) {
  CHECK_EQ(select_and_scatter->operand_count(), 3);
  const auto operand = select_and_scatter->operand(0);
  const auto source = select_and_scatter->operand(1);
  const auto init_value = select_and_scatter->operand(2);
  const Window& window = select_and_scatter->window();
  PrimitiveType operand_element_type = operand->shape().element_type();
  const int64 rank = ShapeUtil::Rank(operand->shape());
  CHECK_EQ(rank, ShapeUtil::Rank(source->shape()));
  CHECK_EQ(rank, window.dimensions_size());

  // TODO(b/31410564): Implement dilation for select-and-scatter.
  if (window_util::HasDilation(window)) {
    return Unimplemented(
        "Dilation for SelectAndScatter is not implemented on CPU. ");
  }

  // Pseudo code for select-and-scatter:
  //
  // initialized_flag is initially off for every window, and is turned on after
  // the first iteration is completed and the first operand value is selected.
  //
  // output(*) = init_value
  // for (coordinates S in the source) {
  //   initialized_flag = false
  //   for (coordinates W in the window) {
  //     I = S * stride + W - pad_low
  //     if I within bounds of operand:
  //       if !initialized_flag or select(selected_value, operand(I)) == false:
  //         selected_value = operand(I)
  //         selected_index = I
  //         initialized_flag = true
  //   }
  //   output(selected_index) = scatter(output(selected_index), source(S))
  // }
  //

  // Initialize the output array with the given init_value.
  TF_RETURN_IF_ERROR(EmitTargetElementLoop(
      select_and_scatter, /*desc=*/IrName(select_and_scatter, "init"),
      [this, init_value](const llvm_ir::IrArray::Index& target_index) {
        llvm::Value* init_value_addr = GetEmittedValueFor(init_value);
        return Load(init_value_addr);
      }));

  // Create a loop to iterate over the source array to scatter to the output.
  llvm_ir::ForLoopNest source_loops(IrName(select_and_scatter), &b_);
  const llvm_ir::IrArray::Index source_index =
      source_loops.AddLoopsForShape(source->shape(), "source");
  SetToFirstInsertPoint(source_loops.GetInnerLoopBodyBasicBlock(), &b_);

  // Allocate space to keep the currently selected value, its index, and
  // the boolean initialized_flag, which is initially set to false.
  llvm::Value* selected_value_address = llvm_ir::EmitAllocaAtFunctionEntry(
      llvm_ir::PrimitiveTypeToIrType(operand_element_type, module_),
      "selected_value_address", &b_,
      MinimumAlignmentForPrimitiveType(operand_element_type));
  llvm::Value* selected_index_address =
      llvm_ir::EmitAllocaAtFunctionEntryWithCount(
          b_.getInt64Ty(), b_.getInt32(rank), "selected_index_address", &b_);
  llvm::Value* initialized_flag_address = llvm_ir::EmitAllocaAtFunctionEntry(
      b_.getInt1Ty(), "initialized_flag_address", &b_);
  Store(b_.getInt1(false), initialized_flag_address);

  // Create the inner loop to iterate over the window.
  llvm_ir::ForLoopNest window_loops(IrName(select_and_scatter, "window"), &b_);
  std::vector<int64> window_size;
  for (const auto& dim : window.dimensions()) {
    window_size.push_back(dim.size());
  }
  const llvm_ir::IrArray::Index window_index = window_loops.AddLoopsForShape(
      ShapeUtil::MakeShape(operand_element_type, window_size), "window");
  SetToFirstInsertPoint(window_loops.GetInnerLoopBodyBasicBlock(), &b_);

  // Compute the operand index to visit and evaluate the condition whether the
  // operand index is within the bounds. The unsigned comparison includes
  // checking whether the operand index >= 0.
  llvm_ir::IrArray::Index operand_index(b_.getInt64Ty(), source_index.size());
  llvm::Value* in_bounds_condition = b_.getTrue();
  for (int64 i = 0; i < rank; ++i) {
    llvm::Value* strided_index =
        NSWMul(source_index[i], b_.getInt64(window.dimensions(i).stride()));
    operand_index[i] = NSWSub(NSWAdd(strided_index, window_index[i]),
                              b_.getInt64(window.dimensions(i).padding_low()));
    llvm::Value* index_condition =
        ICmpULT(operand_index[i],
                b_.getInt64(ShapeUtil::GetDimension(operand->shape(), i)));
    in_bounds_condition = And(in_bounds_condition, index_condition);
  }
  CHECK(in_bounds_condition != nullptr);

  // Only need to do something if the operand index is within the bounds. First
  // check if the initialized_flag is set.
  llvm_ir::LlvmIfData if_in_bounds =
      llvm_ir::EmitIfThenElse(in_bounds_condition, "in-bounds", &b_);
  SetToFirstInsertPoint(if_in_bounds.true_block, &b_);
  llvm_ir::LlvmIfData if_initialized = llvm_ir::EmitIfThenElse(
      Load(initialized_flag_address), "initialized", &b_);

  // If the initialized_flag is false, initialize the selected value and index
  // with the currently visiting operand.
  SetToFirstInsertPoint(if_initialized.false_block, &b_);
  const auto save_operand_index =
      [&](const llvm_ir::IrArray::Index& operand_index) {
        for (int64 i = 0; i < rank; ++i) {
          llvm::Value* selected_index_address_slot =
              InBoundsGEP(selected_index_address, {b_.getInt32(i)});
          Store(operand_index[i], selected_index_address_slot);
        }
      };
  llvm_ir::IrArray operand_array(GetIrArrayFor(operand));
  llvm::Value* operand_data =
      operand_array.EmitReadArrayElement(operand_index, &b_);
  Store(operand_data, selected_value_address);
  save_operand_index(operand_index);
  Store(b_.getInt1(true), initialized_flag_address);

  // If the initialized_flag is true, call the `select` function to potentially
  // update the selected value and index with the currently visiting operand.
  SetToFirstInsertPoint(if_initialized.true_block, &b_);
  llvm::Value* operand_address =
      operand_array.EmitArrayElementAddress(operand_index, &b_);
  llvm::Value* operand_element = Load(operand_address);
  llvm::Value* result = EmitThreadLocalCall(
      *select_and_scatter->select(),
      {Load(selected_value_address), operand_element}, "select_function");

  // If the 'select' function returns false, update the selected value and the
  // index to the currently visiting operand.
  llvm::Value* cond = ICmpNE(
      result,
      llvm::ConstantInt::get(llvm_ir::PrimitiveTypeToIrType(PRED, module_), 0),
      "boolean_predicate");
  llvm_ir::LlvmIfData if_select_lhs =
      llvm_ir::EmitIfThenElse(cond, "if-select-lhs", &b_);
  SetToFirstInsertPoint(if_select_lhs.false_block, &b_);
  Store(Load(operand_address), selected_value_address);
  save_operand_index(operand_index);

  // After iterating over the window elements, scatter the source element to
  // the selected index of the output. The value we store at the output
  // location is computed by calling the `scatter` function with the source
  // value and the current output value.
  SetToFirstInsertPoint(window_loops.GetOuterLoopExitBasicBlock(), &b_);
  llvm_ir::IrArray::Index selected_index(source_index.GetType());
  for (int64 i = 0; i < rank; ++i) {
    llvm::Value* selected_index_address_slot =
        InBoundsGEP(selected_index_address, {b_.getInt32(i)});
    selected_index.push_back(Load(selected_index_address_slot));
  }
  llvm_ir::IrArray source_array(GetIrArrayFor(source));
  llvm::Value* source_value =
      source_array.EmitReadArrayElement(source_index, &b_);
  llvm_ir::IrArray output_array(GetIrArrayFor(select_and_scatter));
  llvm::Value* output_value =
      output_array.EmitReadArrayElement(selected_index, &b_);
  llvm::Value* scatter_value =
      EmitThreadLocalCall(*select_and_scatter->scatter(),
                          {output_value, source_value}, "scatter_function");
  output_array.EmitWriteArrayElement(selected_index, scatter_value, &b_);

  SetToFirstInsertPoint(source_loops.GetOuterLoopExitBasicBlock(), &b_);
  return Status::OK();
}

Status IrEmitter::HandleDot(HloInstruction* dot) {
  auto lhs = dot->operand(0);
  auto rhs = dot->operand(1);
  TF_RETURN_IF_ERROR(ElementTypesSameAndSupported(
      /*instruction=*/*dot, /*operands=*/{lhs, rhs},
      /*supported_types=*/{F16, F32, F64, C64}));
  const DotDimensionNumbers& dnums = dot->dot_dimension_numbers();
  if (dnums.lhs_batch_dimensions_size() > 0 ||
      dnums.rhs_batch_dimensions_size() > 0) {
    return Unimplemented("Dot with batch dimensions not implemented.");
  }

  if (dnums.lhs_contracting_dimensions_size() != 1) {
    // This is disallowed by ShapeInference today.
    return Unimplemented(
        "Dot with multiple contracting dimensions not implemented.");
  }

  llvm_ir::IrArray lhs_array(GetIrArrayFor(lhs));
  llvm_ir::IrArray rhs_array(GetIrArrayFor(rhs));

  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(dot));
  llvm_ir::IrArray target_array = GetIrArrayFor(dot);

  VLOG(2) << "HandleDot: ";
  VLOG(2) << "  lhs operand: "
          << llvm_ir::DumpToString(*lhs_array.GetBasePointer());
  VLOG(2) << "  rhs operand: "
          << llvm_ir::DumpToString(*rhs_array.GetBasePointer());
  VLOG(2) << "  target: "
          << llvm_ir::DumpToString(*target_array.GetBasePointer());

  // Dot operation is complicated so we delegate to a helper class.
  return DotOpEmitter::EmitDotOperation(
      *dot, target_array, lhs_array, rhs_array, /*addend_array=*/nullptr,
      GetExecutableRunOptionsArgument(), &b_, hlo_module_config_,
      target_machine_features_);
}

StatusOr<llvm::Value*> IrEmitter::EmitTargetElementLoopBodyForConvolution(
    HloConvolutionInstruction* convolution,
    const llvm_ir::IrArray::Index& index) {
  const HloInstruction* lhs = convolution->operand(0);
  const HloInstruction* rhs = convolution->operand(1);
  const Window& window = convolution->window();

  const ConvolutionDimensionNumbers& dnums =
      convolution->convolution_dimension_numbers();
  int num_spatial_dims = dnums.output_spatial_dimensions_size();
  std::vector<llvm::Value*> output_spatial(num_spatial_dims);
  for (int i = 0; i < num_spatial_dims; ++i) {
    output_spatial[i] = index[dnums.output_spatial_dimensions(i)];
  }
  llvm::Value* output_feature = index[dnums.output_feature_dimension()];
  llvm::Value* batch = index[dnums.output_batch_dimension()];

  // We will accumulate the products into this sum to calculate the output entry
  // at the given index.
  PrimitiveType lhs_element_type = lhs->shape().element_type();
  llvm::Type* lhs_llvm_type =
      llvm_ir::PrimitiveTypeToIrType(lhs_element_type, module_);
  llvm::Value* sum_address = llvm_ir::EmitAllocaAtFunctionEntry(
      lhs_llvm_type, "convolution_sum_address", &b_,
      MinimumAlignmentForPrimitiveType(lhs_element_type));
  llvm::Value* constant_zero = llvm::Constant::getNullValue(lhs_llvm_type);
  Store(constant_zero, sum_address);

  llvm_ir::ForLoopNest loops(IrName(convolution, "inner"), &b_);
  std::vector<llvm::Value*> kernel_spatial(num_spatial_dims);
  for (int i = 0; i < num_spatial_dims; ++i) {
    kernel_spatial[i] =
        loops
            .AddLoop(
                0, rhs->shape().dimensions(dnums.kernel_spatial_dimensions(i)),
                absl::StrCat("k", i))
            ->GetIndVarValue();
  }
  llvm::Value* input_feature =
      loops
          .AddLoop(0, lhs->shape().dimensions(dnums.input_feature_dimension()),
                   "iz")
          ->GetIndVarValue();

  SetToFirstInsertPoint(loops.GetInnerLoopBodyBasicBlock(), &b_);

  // Calculate the spatial index in the input array, taking striding, dilation
  // and padding into account. An index in the padding will be out of the bounds
  // of the array.
  const auto calculate_input_index = [this](llvm::Value* output_index,
                                            llvm::Value* kernel_index,
                                            const WindowDimension& window_dim) {
    llvm::Value* strided_index =
        NSWMul(output_index, b_.getInt64(window_dim.stride()));
    llvm::Value* dilated_kernel_index =
        NSWMul(kernel_index, b_.getInt64(window_dim.window_dilation()));
    return NSWSub(NSWAdd(strided_index, dilated_kernel_index),
                  b_.getInt64(window_dim.padding_low()));
  };
  std::vector<llvm::Value*> input_spatial(num_spatial_dims);
  for (int i = 0; i < num_spatial_dims; ++i) {
    input_spatial[i] = calculate_input_index(
        output_spatial[i], kernel_spatial[i], window.dimensions(i));
  }

  // We need to check if 0 <= input dim < bound, as otherwise we are in the
  // padding so that we can skip the computation. That is equivalent to input
  // dim < bound as an *unsigned* comparison, since a negative value will wrap
  // to a large positive value. The input dim is dilated, so we need to dilate
  // the bound as well to match.

  // Also need to check that the input coordinates are not in one of the
  // holes created by base dilation.
  const auto not_in_hole = [&](llvm::Value* input_index, int64 base_dilation) {
    llvm::Value* remainder = SRem(input_index, b_.getInt64(base_dilation));
    return ICmpEQ(remainder, b_.getInt64(0));
  };

  llvm::Value* in_bounds_condition = b_.getInt1(true);
  for (int i = 0; i < num_spatial_dims; ++i) {
    llvm::ConstantInt* input_bound = b_.getInt64(window_util::DilatedBound(
        lhs->shape().dimensions(dnums.input_spatial_dimensions(i)),
        window.dimensions(i).base_dilation()));
    llvm::Value* dim_in_bound = ICmpULT(input_spatial[i], input_bound);
    llvm::Value* dim_not_in_hole =
        not_in_hole(input_spatial[i], window.dimensions(i).base_dilation());
    llvm::Value* dim_ok = And(dim_in_bound, dim_not_in_hole);
    in_bounds_condition = And(in_bounds_condition, dim_ok);
  }

  // Now we need to map the dilated base coordinates back to the actual
  // data indices on the lhs.
  const auto undilate = [&](llvm::Value* input_index, int64 base_dilation) {
    return SDiv(input_index, b_.getInt64(base_dilation));
  };
  for (int i = 0; i < num_spatial_dims; ++i) {
    input_spatial[i] =
        undilate(input_spatial[i], window.dimensions(i).base_dilation());
  }

  llvm_ir::LlvmIfData if_data =
      llvm_ir::EmitIfThenElse(in_bounds_condition, "in-bounds", &b_);
  SetToFirstInsertPoint(if_data.true_block, &b_);

  // We are not in the padding, so carry out the computation.
  int num_dims = num_spatial_dims + 2;
  llvm_ir::IrArray::Index input_index(b_.getInt64Ty(), num_dims);
  for (int i = 0; i < num_spatial_dims; ++i) {
    input_index[dnums.input_spatial_dimensions(i)] = input_spatial[i];
  }
  input_index[dnums.input_feature_dimension()] = input_feature;
  input_index[dnums.input_batch_dimension()] = batch;

  llvm_ir::IrArray kernel_array(GetIrArrayFor(rhs));
  llvm_ir::IrArray::Index kernel_index(b_.getInt64Ty(), num_dims);
  for (int i = 0; i < num_spatial_dims; ++i) {
    kernel_index[dnums.kernel_spatial_dimensions(i)] =
        window.dimensions(i).window_reversal()
            ? NSWSub(b_.getInt64(window.dimensions(i).size() - 1),
                     kernel_spatial[i])
            : kernel_spatial[i];
  }

  kernel_index[dnums.kernel_input_feature_dimension()] = input_feature;
  kernel_index[dnums.kernel_output_feature_dimension()] = output_feature;

  llvm_ir::IrArray input_array(GetIrArrayFor(lhs));
  llvm::Value* product =
      FMul(input_array.EmitReadArrayElement(input_index, &b_),
           kernel_array.EmitReadArrayElement(kernel_index, &b_));
  llvm::Value* sum = FAdd(Load(sum_address), product);
  Store(sum, sum_address);

  SetToFirstInsertPoint(loops.GetOuterLoopExitBasicBlock(), &b_);
  return Load(sum_address);
}

Status IrEmitter::HandleConvolution(HloInstruction* convolution) {
  auto lhs = convolution->operand(0);
  auto rhs = convolution->operand(1);
  TF_RETURN_IF_ERROR(ElementTypesSameAndSupported(
      /*instruction=*/*convolution, /*operands=*/{lhs, rhs},
      /*supported_types=*/{F16, F32, C64}));

  // TODO(tonywy): Add PotentiallyImplementedAsMKLCovolution to support
  // different data layouts.
  if (PotentiallyImplementedAsEigenConvolution(*convolution,
                                               target_machine_features_)) {
    const Shape& lhs_shape = lhs->shape();
    const Shape& rhs_shape = rhs->shape();
    const Shape& convolution_shape = convolution->shape();
    // The input, kernel and output agree with respect to layout.
    if (LayoutUtil::IsMonotonicWithDim0Major(lhs_shape.layout()) &&
        LayoutUtil::IsMonotonicWithDim0Major(rhs_shape.layout()) &&
        LayoutUtil::IsMonotonicWithDim0Major(convolution_shape.layout())) {
      // We lower 1D convolutions into calls to the same Eigen function as 2D
      // convolutions, except that we pretend that the 1D convolution is really
      // a 2D convolution with the missing dimension set to 1.  We also adjust
      // the padding, dilation parameters as needed.
      bool one_dim_convolution = lhs_shape.dimensions_size() == 3;
      llvm::Value* lhs_address = GetEmittedValueFor(lhs);
      llvm::Value* rhs_address = GetEmittedValueFor(rhs);
      TF_RETURN_IF_ERROR(EmitTargetAddressForOp(convolution));

      const ConvolutionDimensionNumbers& dnums =
          convolution->convolution_dimension_numbers();

      // Input tensor.
      const Shape& input_shape = convolution->operand(0)->shape();
      int64 input_batch = input_shape.dimensions(dnums.input_batch_dimension());
      int64 input_rows =
          input_shape.dimensions(dnums.input_spatial_dimensions(0));
      int64 input_cols =
          one_dim_convolution
              ? 1
              : input_shape.dimensions(dnums.input_spatial_dimensions(1));
      int64 input_channels =
          input_shape.dimensions(dnums.input_feature_dimension());

      // Kernel tensor.
      const Shape& kernel_shape = convolution->operand(1)->shape();
      int64 kernel_rows =
          kernel_shape.dimensions(dnums.kernel_spatial_dimensions(0));
      int64 kernel_cols =
          one_dim_convolution
              ? 1
              : kernel_shape.dimensions(dnums.kernel_spatial_dimensions(1));
      int64 kernel_channels =
          kernel_shape.dimensions(dnums.kernel_input_feature_dimension());
      int64 kernel_filters =
          kernel_shape.dimensions(dnums.kernel_output_feature_dimension());

      // Output tensor.
      const Shape& convolution_shape = convolution->shape();
      int64 output_rows =
          convolution_shape.dimensions(dnums.output_spatial_dimensions(0));
      int64 output_cols = one_dim_convolution
                              ? 1
                              : convolution_shape.dimensions(
                                    dnums.output_spatial_dimensions(1));

      // Extract the window stride for the convolution.
      const Window& window = convolution->window();
      int64 row_stride = window.dimensions(0).stride();
      int64 col_stride =
          one_dim_convolution ? 1 : window.dimensions(1).stride();

      int64 padding_top = window.dimensions(0).padding_low();
      int64 padding_bottom = window.dimensions(0).padding_high();
      int64 padding_left =
          one_dim_convolution ? 0 : window.dimensions(1).padding_low();
      int64 padding_right =
          one_dim_convolution ? 0 : window.dimensions(1).padding_high();

      int64 lhs_row_dilation = window.dimensions(0).base_dilation();
      int64 lhs_col_dilation =
          one_dim_convolution ? 1 : window.dimensions(1).base_dilation();
      int64 rhs_row_dilation = window.dimensions(0).window_dilation();
      int64 rhs_col_dilation =
          one_dim_convolution ? 1 : window.dimensions(1).window_dilation();

      PrimitiveType primitive_type = lhs->shape().element_type();
      llvm::Type* ir_ptr_type = primitive_type == F16
                                    ? b_.getHalfTy()->getPointerTo()
                                    : b_.getFloatTy()->getPointerTo();
      llvm::Type* int64_type = b_.getInt64Ty();
      llvm::Type* int8_ptr_type = b_.getInt8Ty()->getPointerTo();
      llvm::FunctionType* conv_type = llvm::FunctionType::get(
          b_.getVoidTy(),
          {int8_ptr_type, ir_ptr_type, ir_ptr_type, ir_ptr_type, int64_type,
           int64_type,    int64_type,  int64_type,  int64_type,  int64_type,
           int64_type,    int64_type,  int64_type,  int64_type,  int64_type,
           int64_type,    int64_type,  int64_type,  int64_type,  int64_type,
           int64_type,    int64_type,  int64_type,  int64_type},
          /*isVarArg=*/false);
      bool multi_threaded =
          hlo_module_config_.debug_options().xla_cpu_multi_thread_eigen();
      bool use_mkl_dnn =
          hlo_module_config_.debug_options().xla_cpu_use_mkl_dnn();

      // TODO(b/78639006) Singlethread MKL conv2d is not implemented due to the
      // potential race condition by setting the omp_num_threads.
      const char* fn_name =
          primitive_type == F16
              ? (multi_threaded
                     ? runtime::kEigenConvF16SymbolName
                     : runtime::kEigenSingleThreadedConvF16SymbolName)
              : (multi_threaded
                     ? (use_mkl_dnn ? runtime::kMKLConvF32SymbolName
                                    : runtime::kEigenConvF32SymbolName)
                     : runtime::kEigenSingleThreadedConvF32SymbolName);
      if (!multi_threaded && use_mkl_dnn) {
        LOG(WARNING) << "Using Eigen instead of MKL-DNN for single-threaded "
                        "conv2d function.";
      }
      llvm::Function* conv_func = llvm::cast<llvm::Function>(
          module_->getOrInsertFunction(fn_name, conv_type));
      conv_func->setCallingConv(llvm::CallingConv::C);
      conv_func->setDoesNotThrow();
      conv_func->setOnlyAccessesArgMemory();
      Call(conv_func, {
                          GetExecutableRunOptionsArgument(),
                          BitCast(GetEmittedValueFor(convolution), ir_ptr_type),
                          BitCast(lhs_address, ir_ptr_type),
                          BitCast(rhs_address, ir_ptr_type),
                          b_.getInt64(input_batch),
                          b_.getInt64(input_rows),
                          b_.getInt64(input_cols),
                          b_.getInt64(input_channels),
                          b_.getInt64(kernel_rows),
                          b_.getInt64(kernel_cols),
                          b_.getInt64(kernel_channels),
                          b_.getInt64(kernel_filters),
                          b_.getInt64(output_rows),
                          b_.getInt64(output_cols),
                          b_.getInt64(row_stride),
                          b_.getInt64(col_stride),
                          b_.getInt64(padding_top),
                          b_.getInt64(padding_bottom),
                          b_.getInt64(padding_left),
                          b_.getInt64(padding_right),
                          b_.getInt64(lhs_row_dilation),
                          b_.getInt64(lhs_col_dilation),
                          b_.getInt64(rhs_row_dilation),
                          b_.getInt64(rhs_col_dilation),
                      });

      return Status::OK();
    }
  }

  // This is a completely un-optimized version of convolution just to
  // have an early version that works. E.g. the input index and
  // padding calculation is not hoisted out of the inner loop.
  //
  // See the description of convolution in the XLA documentation for the pseudo
  // code for convolution.
  return EmitTargetElementLoop(
      convolution, [&](const llvm_ir::IrArray::Index& index) {
        return EmitTargetElementLoopBodyForConvolution(
            Cast<HloConvolutionInstruction>(convolution), index);
      });
}

Status IrEmitter::HandleFft(HloInstruction* fft) {
  auto operand = fft->operand(0);
  TF_RETURN_IF_ERROR(ElementTypesSameAndSupported(
      /*instruction=*/*fft, /*operands=*/{operand},
      /*supported_types=*/{F32, C64}));
  TF_RET_CHECK(LayoutUtil::IsMonotonicWithDim0Major(operand->shape().layout()));
  TF_RET_CHECK(LayoutUtil::IsMonotonicWithDim0Major(fft->shape().layout()));
  VLOG(3) << "operand=" << ShapeUtil::HumanStringWithLayout(operand->shape());
  VLOG(3) << "fft=" << ShapeUtil::HumanStringWithLayout(fft->shape());

  llvm::Value* operand_address = GetEmittedValueFor(operand);
  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(fft));

  const std::vector<int64>& fft_length = fft->fft_length();
  int64 input_batch = 1;
  for (int i = 0; i < fft->shape().dimensions_size() - fft_length.size(); i++) {
    input_batch *= fft->shape().dimensions(i);
  }

  // Args have been computed, make the call.
  llvm::Type* int8_ptr_type = b_.getInt8Ty()->getPointerTo();
  llvm::Type* int32_type = b_.getInt32Ty();
  llvm::Type* int64_type = b_.getInt64Ty();
  llvm::FunctionType* fft_type = llvm::FunctionType::get(
      b_.getVoidTy(),
      {int8_ptr_type, int8_ptr_type, int8_ptr_type, int32_type, int32_type,
       int64_type, int64_type, int64_type, int64_type},
      /*isVarArg=*/false);

  bool multi_threaded_eigen =
      hlo_module_config_.debug_options().xla_cpu_multi_thread_eigen();
  const char* fn_name = multi_threaded_eigen
                            ? runtime::kEigenFftSymbolName
                            : runtime::kEigenSingleThreadedFftSymbolName;

  llvm::Function* fft_func = llvm::cast<llvm::Function>(
      module_->getOrInsertFunction(fn_name, fft_type));
  fft_func->setCallingConv(llvm::CallingConv::C);
  fft_func->setDoesNotThrow();
  fft_func->setOnlyAccessesInaccessibleMemOrArgMem();
  const int fft_rank = fft_length.size();
  Call(fft_func,
       {GetExecutableRunOptionsArgument(),
        BitCast(GetEmittedValueFor(fft), int8_ptr_type),
        BitCast(operand_address, int8_ptr_type), b_.getInt32(fft->fft_type()),
        b_.getInt32(fft_rank), b_.getInt64(input_batch),
        b_.getInt64(fft_rank > 0 ? fft_length[0] : 0),
        b_.getInt64(fft_rank > 1 ? fft_length[1] : 0),
        b_.getInt64(fft_rank > 2 ? fft_length[2] : 0)});

  return Status::OK();
}

Status IrEmitter::HandleCrossReplicaSum(HloInstruction* crs) {
  if (hlo_module_config_.replica_count() != 1) {
    // TODO(b/33011107): Support nontrivial cross replica sum on CPU.
    return Unimplemented(
        "CrossReplicaSum with >1 replica is not implemented on CPU.");
  }

  // When there is a single replica, a cross replica sum is the identity
  // function, and the buffer assignment expects a copy.
  //
  // TODO(b/80100934): We would like to eliminate one-replica CRS nodes entirely
  // in algebraic-simplifier, but currently on some platforms
  // HloModuleConfig::num_replicas changes between when the module is compiled
  // and when it's run.
  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(crs));

  // CRS with one operand and one replica is simply the identity function.
  if (crs->operand_count() == 1) {
    return EmitMemcpy(*crs->operand(0), *crs);
  }

  // CRS with multiple operands and one replica produces a (one-deep) tuple.
  std::vector<llvm::Value*> operand_ptrs;
  for (int64 i = 0; i < crs->operand_count(); ++i) {
    llvm::Value* in_ptr = GetEmittedValueFor(crs->operand(i));
    TF_ASSIGN_OR_RETURN(const BufferAllocation::Slice out_slice,
                        assignment_.GetUniqueSlice(crs, {i}));

    const Shape& operand_shape = crs->operand(i)->shape();
    CHECK(ShapeUtil::IsArray(operand_shape))
        << "Operands to cross-replica-sum must be arrays: " << crs->ToString();
    operand_ptrs.push_back(EmitBufferPointer(out_slice, operand_shape));

    // TODO(b/63762267): Be more aggressive about specifying alignment.
    MemCpy(operand_ptrs.back(), /*DstAlign=*/1, in_ptr,
           /*SrcAlign=*/1, ShapeUtil::ByteSizeOf(operand_shape));
  }
  llvm_ir::EmitTuple(GetIrArrayFor(crs), operand_ptrs, &b_, module_);
  return Status::OK();
}

Status IrEmitter::HandleParameter(HloInstruction* parameter) {
  VLOG(2) << "HandleParameter: " << parameter->ToString();
  return EmitTargetAddressForOp(parameter);
}

// Returns true if the relative order of the unreduced dimensions stays the same
// through the reduce operation.
static bool ReductionPreservesLayout(const HloInstruction& reduce) {
  DCHECK_EQ(reduce.opcode(), HloOpcode::kReduce);

  // Maps dimensions that were not reduced from their dimension numbers in the
  // source shape to their dimensions numbers in the destination shape.
  //
  // So if we reduce f32[A,B,C,D] on dimensions 1 and 2, this map contains
  // [0->0, 3->1].
  absl::flat_hash_map<int64, int64> unreduced_dim_map;

  absl::flat_hash_set<int64> reduced_dims(reduce.dimensions().begin(),
                                          reduce.dimensions().end());

  const Shape& operand_shape = reduce.operand(0)->shape();
  const Shape& result_shape = reduce.shape();

  int64 delta = 0;
  for (int64 i = 0; i < operand_shape.dimensions_size(); i++) {
    if (reduced_dims.count(i)) {
      delta++;
    } else {
      InsertOrDie(&unreduced_dim_map, i, i - delta);
    }
  }

  // Iterate dimensions minor to major and check that the corresponding
  // dimensions in the source and target shapes are equivalent.
  int64 result_dim_idx = 0;
  for (int64 operand_dim_idx = 0;
       operand_dim_idx < operand_shape.dimensions_size(); operand_dim_idx++) {
    int64 operand_dim = operand_shape.layout().minor_to_major(operand_dim_idx);
    if (!reduced_dims.count(operand_dim)) {
      if (FindOrDie(unreduced_dim_map, operand_dim) !=
          result_shape.layout().minor_to_major(result_dim_idx++)) {
        return false;
      }
    }
  }

  CHECK_EQ(result_dim_idx, result_shape.dimensions_size());

  return true;
}

IrEmitter::ReductionGenerator IrEmitter::MatchReductionGenerator(
    HloComputation* function, string* failure_reason) const {
  CHECK_EQ(function->num_parameters(), 2);

  auto root_instruction = function->root_instruction();
  CHECK(ShapeUtil::IsScalar(root_instruction->shape()));

  if (root_instruction->operand_count() != 2) {
    *failure_reason = "root instruction is not a binary operation";
    return nullptr;
  }

  const Shape& root_shape = root_instruction->shape();
  if (ShapeUtil::ElementIsComplex(root_shape)) {
    // TODO(b/65408531): Complex add could by done via bitcast to <float x [2N]>
    // Complex multiply would be more challenging. We could perhaps use a
    // strided load to get all reals in a vector, all images in a vector, or use
    // CreateShuffleVector on a bitcast to float x [2N].
    *failure_reason = "complex values not supported";
    return nullptr;
  }
  bool root_is_floating_point = ShapeUtil::ElementIsFloating(root_shape);
  bool root_is_integral = ShapeUtil::ElementIsIntegral(root_shape);
  bool root_is_signed = ShapeUtil::ElementIsSigned(root_shape);

  auto lhs = root_instruction->operand(0);
  auto rhs = root_instruction->operand(1);

  auto param_0 = function->parameter_instruction(0);
  auto param_1 = function->parameter_instruction(1);
  if (!(lhs == param_0 && rhs == param_1) &&
      !(rhs == param_0 && lhs == param_1)) {
    *failure_reason =
        "root instruction is not a binary operation on the incoming arguments";
    return nullptr;
  }

  CHECK(ShapeUtil::IsScalar(lhs->shape()) && ShapeUtil::IsScalar(rhs->shape()));

  // This is visually similar to ElementalIrEmitter, though conceptually we're
  // doing something different here.  ElementalIrEmitter emits scalar operations
  // while these emit scalar or vector operations depending on the type of the
  // operands. See CreateShardedVectorType for the actual types in use here.
  switch (root_instruction->opcode()) {
    default:
      *failure_reason = "did not recognize root instruction opcode";
      return nullptr;

    case HloOpcode::kAdd:
      return [root_is_integral](llvm::IRBuilder<>* b, llvm::Value* lhs,
                                llvm::Value* rhs) {
        return root_is_integral ? b->CreateAdd(lhs, rhs)
                                : b->CreateFAdd(lhs, rhs);
      };

    case HloOpcode::kMultiply:
      return [root_is_integral](llvm::IRBuilder<>* b, llvm::Value* lhs,
                                llvm::Value* rhs) {
        return root_is_integral ? b->CreateMul(lhs, rhs)
                                : b->CreateFMul(lhs, rhs);
      };

    case HloOpcode::kAnd:
      return [](llvm::IRBuilder<>* b, llvm::Value* lhs, llvm::Value* rhs) {
        return b->CreateAnd(lhs, rhs);
      };

    case HloOpcode::kOr:
      return [](llvm::IRBuilder<>* b, llvm::Value* lhs, llvm::Value* rhs) {
        return b->CreateOr(lhs, rhs);
      };

    case HloOpcode::kXor:
      return [](llvm::IRBuilder<>* b, llvm::Value* lhs, llvm::Value* rhs) {
        return b->CreateXor(lhs, rhs);
      };

    case HloOpcode::kMaximum:
      return [root_is_floating_point, root_is_signed](
                 llvm::IRBuilder<>* b, llvm::Value* lhs,
                 llvm::Value* rhs) -> llvm::Value* {
        if (root_is_floating_point) {
          return llvm_ir::EmitCallToIntrinsic(llvm::Intrinsic::maxnum,
                                              {lhs, rhs}, {lhs->getType()}, b);
        }

        return b->CreateSelect(
            b->CreateICmp(root_is_signed ? llvm::ICmpInst::ICMP_SGE
                                         : llvm::ICmpInst::ICMP_UGE,
                          lhs, rhs),
            lhs, rhs);
      };

    case HloOpcode::kMinimum:
      return [root_is_floating_point, root_is_signed](
                 llvm::IRBuilder<>* b, llvm::Value* lhs,
                 llvm::Value* rhs) -> llvm::Value* {
        if (root_is_floating_point) {
          return llvm_ir::EmitCallToIntrinsic(llvm::Intrinsic::minnum,
                                              {lhs, rhs}, {lhs->getType()}, b);
        }

        return b->CreateSelect(
            b->CreateICmp(root_is_signed ? llvm::ICmpInst::ICMP_SLE
                                         : llvm::ICmpInst::ICMP_ULE,
                          lhs, rhs),
            lhs, rhs);
      };
  }
}

IrEmitter::ShardedVectorType IrEmitter::CreateShardedVectorType(
    PrimitiveType element_type, unsigned element_count) {
  int vector_register_size_in_elements =
      target_machine_features_.vector_register_byte_size(
          *compute_function_->function()) /
      ShapeUtil::ByteSizeOfPrimitiveType(element_type);

  ShardedVectorType sharded_vector_type;
  llvm::Type* element_ir_type =
      llvm_ir::PrimitiveTypeToIrType(element_type, module_);

  for (int i = 0, e = 1 + tensorflow::Log2Ceiling(element_count); i < e; i++) {
    // For every power of two present in element_count, we generate one or more
    // vector or scalar types.
    const unsigned current_size_fragment = 1u << i;
    if (!(element_count & current_size_fragment)) {
      // Power of two not present in element_count.
      continue;
    }

    if (current_size_fragment == 1) {
      // Single element, use a scalar type.
      sharded_vector_type.push_back(element_ir_type);
      continue;
    }

    // Lower "current_size_fragment" number of elements using (as few as
    // possible) vector registers.

    if (current_size_fragment >= vector_register_size_in_elements) {
      auto vector_type = llvm::VectorType::get(
          element_ir_type, vector_register_size_in_elements);
      sharded_vector_type.insert(
          sharded_vector_type.end(),
          current_size_fragment / vector_register_size_in_elements,
          vector_type);

      // Both current_size_fragment and vector_register_size_in_elements are
      // powers of two.
      CHECK_EQ(current_size_fragment % vector_register_size_in_elements, 0);
      continue;
    }

    // For now we assume that vector_register_size_in_elements and lower powers
    // of two are all legal vector sizes (or at least can be lowered easily by
    // LLVM).
    sharded_vector_type.push_back(
        llvm::VectorType::get(element_ir_type, current_size_fragment));
  }
  return sharded_vector_type;
}

StatusOr<IrEmitter::ShardedVector>
IrEmitter::EmitInnerLoopForVectorizedReduction(
    const ReductionGenerator& reduction_generator,
    const llvm_ir::IrArray::Index& output_index,
    const ShardedVectorType& accumulator_type, HloInstruction* init_value,
    HloInstruction* arg, absl::Span<const int64> dimensions,
    unsigned element_alignment) {
  ShardedVector accumulator;
  accumulator.reserve(accumulator_type.size());
  for (auto accumulator_shard_type : accumulator_type) {
    accumulator.push_back(llvm_ir::EmitAllocaAtFunctionEntry(
        accumulator_shard_type, "accumulator", &b_, 0));
  }

  llvm::Value* init_value_ssa = Load(GetEmittedValueFor(init_value));

  for (llvm::Value* accumulator_shard : accumulator) {
    llvm::Value* initial_value;
    auto shard_type = accumulator_shard->getType()->getPointerElementType();
    if (auto vector_type = llvm::dyn_cast<llvm::VectorType>(shard_type)) {
      initial_value =
          VectorSplat(vector_type->getNumElements(), init_value_ssa);
    } else {
      initial_value = init_value_ssa;
    }

    AlignedStore(initial_value, accumulator_shard, element_alignment);
  }

  llvm_ir::ForLoopNest reduction_loop_nest(IrName(arg, "vectorized_inner"),
                                           &b_);
  llvm_ir::IrArray::Index reduced_dims_index =
      reduction_loop_nest.AddLoopsForShapeOnDimensions(arg->shape(), dimensions,
                                                       "reduction_dim");

  SetToFirstInsertPoint(reduction_loop_nest.GetInnerLoopBodyBasicBlock(), &b_);

  llvm_ir::IrArray arg_array(GetIrArrayFor(arg));
  llvm_ir::IrArray::Index input_index = reduced_dims_index;
  llvm_ir::IrArray::Index::const_iterator it = output_index.begin();

  for (size_t i = 0; i < input_index.size(); ++i) {
    if (input_index[i] == nullptr) {
      input_index[i] = *it++;
    }
  }
  CHECK(output_index.end() == it);

  llvm::Value* input_address = BitCast(
      arg_array.EmitArrayElementAddress(input_index, &b_), b_.getInt8PtrTy());

  for (int i = 0; i < accumulator.size(); i++) {
    auto input_address_typed =
        BitCast(input_address, accumulator[i]->getType());
    auto current_accumulator_value =
        AlignedLoad(accumulator[i], element_alignment);
    auto addend = AlignedLoad(input_address_typed, element_alignment);
    arg_array.AnnotateLoadStoreInstructionWithMetadata(addend);

    auto reduced_result =
        reduction_generator(&b_, current_accumulator_value, addend);
    AlignedStore(reduced_result, accumulator[i], element_alignment);

    if (i != (accumulator.size() - 1)) {
      input_address = ConstInBoundsGEP1_32(reduced_result->getType(),
                                           input_address_typed, 1);
    }
  }

  SetToFirstInsertPoint(reduction_loop_nest.GetOuterLoopExitBasicBlock(), &b_);

  ShardedVector result_ssa;
  result_ssa.reserve(accumulator.size());
  for (auto accumulator_shard : accumulator) {
    result_ssa.push_back(AlignedLoad(accumulator_shard, element_alignment));
  }
  return result_ssa;
}

void IrEmitter::EmitShardedVectorStore(
    llvm::Value* store_address, const std::vector<llvm::Value*>& value_to_store,
    const int alignment, const llvm_ir::IrArray& containing_array) {
  for (int i = 0; i < value_to_store.size(); i++) {
    auto store_address_typed =
        BitCast(store_address,
                llvm::PointerType::getUnqual(value_to_store[i]->getType()));

    auto store_instruction =
        AlignedStore(value_to_store[i], store_address_typed, alignment);
    containing_array.AnnotateLoadStoreInstructionWithMetadata(
        store_instruction);

    if (i != (value_to_store.size() - 1)) {
      store_address = ConstInBoundsGEP1_32(value_to_store[i]->getType(),
                                           store_address_typed, 1);
    }
  }
}

StatusOr<bool> IrEmitter::EmitVectorizedReduce(
    HloInstruction* reduce, HloInstruction* arg, HloInstruction* init_value,
    absl::Span<const int64> dimensions, HloComputation* function,
    string* failure_reason) {
  if (!ReductionPreservesLayout(*reduce)) {
    return false;
  }

  ReductionGenerator reduction_generator =
      MatchReductionGenerator(function, failure_reason);
  if (!reduction_generator) {
    return false;
  }

  int vectorization_factor_in_bytes =
      target_machine_features_.vectorization_factor_in_bytes();

  // We try to process vectorization_factor elements at the same time.
  const int vectorization_factor =
      vectorization_factor_in_bytes /
      ShapeUtil::ByteSizeOfPrimitiveType(reduce->shape().element_type());

  bool is_reduction_over_minor_dimension =
      std::find(dimensions.begin(), dimensions.end(),
                LayoutUtil::Minor(arg->shape().layout(), 0)) !=
      dimensions.end();

  unsigned element_alignment = tensorflow::MathUtil::GCD<unsigned>(
      ShapeUtil::ByteSizeOfPrimitiveType(reduce->shape().element_type()),
      MinimumAlignmentForPrimitiveType(reduce->shape().element_type()));

  if (is_reduction_over_minor_dimension) {
    // TODO(sanjoy): Implement vectorized reduction over the minor dimension.
    *failure_reason = "reduction over minor dimension not implemented";
    return false;
  }

  CHECK(!ShapeUtil::IsTuple(reduce->shape()));
  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(reduce));

  // We know we're not reducing over the most minor dimension, which means we
  // can lower the reduction loop as:
  //
  //  1. We're reducing over dimensions R0, R1.
  //  2. D0 is the most minor dimension.
  //  3. VS is the vectorization stride (we want to reduce this many elements at
  //     once)
  //
  //  for (d1 in D1) {
  //    for (d0 in D0 with stride VS) {
  //      vector_acc = init
  //      for (r1 in R1) {
  //        for (r0 in R0) {
  //          vector_acc = elementwise_reduce(vector_acc, input[d1, d0, r1, r0]
  //        }
  //      }
  //      output[d1, d0] = vector_acc
  //    }
  //  }

  llvm_ir::ForLoopNest loop_nest(IrName(reduce), &b_);
  llvm_ir::IrArray::Index array_index(b_.getInt64Ty(),
                                      reduce->shape().dimensions_size());
  for (int i = LayoutUtil::MinorToMajor(reduce->shape()).size() - 1; i > 0;
       --i) {
    int64 dimension = LayoutUtil::Minor(reduce->shape().layout(), i);
    int64 start_index = 0;
    int64 end_index = reduce->shape().dimensions(dimension);
    std::unique_ptr<llvm_ir::ForLoop> loop = loop_nest.AddLoop(
        start_index, end_index, absl::StrFormat("dim.%d", dimension));
    array_index[dimension] = loop->GetIndVarValue();
  }

  int64 innermost_dimension = LayoutUtil::Minor(reduce->shape().layout(), 0);
  int64 innermost_dimension_size =
      reduce->shape().dimensions(innermost_dimension);

  if (llvm::BasicBlock* innermost_body_bb =
          loop_nest.GetInnerLoopBodyBasicBlock()) {
    SetToFirstInsertPoint(innermost_body_bb, &b_);
  }

  auto outermost_loop_exit_block = loop_nest.GetOuterLoopExitBasicBlock();

  if (innermost_dimension_size >= vectorization_factor) {
    int64 start_index = 0;
    int64 end_index = (innermost_dimension_size / vectorization_factor) *
                      vectorization_factor;
    std::unique_ptr<llvm_ir::ForLoop> loop =
        loop_nest.AddLoop(start_index, end_index, vectorization_factor,
                          absl::StrFormat("dim.%d", innermost_dimension));
    array_index[innermost_dimension] = loop->GetIndVarValue();

    SetToFirstInsertPoint(loop->GetBodyBasicBlock(), &b_);

    ShardedVectorType vector_type = CreateShardedVectorType(
        reduce->shape().element_type(), vectorization_factor);
    TF_ASSIGN_OR_RETURN(std::vector<llvm::Value*> accumulator,
                        EmitInnerLoopForVectorizedReduction(
                            reduction_generator, array_index, vector_type,
                            init_value, arg, dimensions, element_alignment));

    llvm_ir::IrArray target_array = GetIrArrayFor(reduce);
    llvm::Value* output_address =
        target_array.EmitArrayElementAddress(array_index, &b_);
    EmitShardedVectorStore(output_address, accumulator, element_alignment,
                           target_array);

    if (auto exit_terminator = loop->GetExitBasicBlock()->getTerminator()) {
      CHECK_GT(LayoutUtil::MinorToMajor(reduce->shape()).size(), 1);
      b_.SetInsertPoint(exit_terminator);
    } else {
      CHECK_EQ(LayoutUtil::MinorToMajor(reduce->shape()).size(), 1);
      b_.SetInsertPoint(loop->GetExitBasicBlock());
    }
  }

  // Since we increment the stride for the inner dimension by more than 1, we
  // may need to peel out an "epilogue" iteration to get the remaining elements
  // in the following case:
  if (innermost_dimension_size % vectorization_factor) {
    // TODO(b/63775531): Consider using a scalar loop here to save on code size.
    array_index[innermost_dimension] =
        b_.getInt64(innermost_dimension_size -
                    (innermost_dimension_size % vectorization_factor));

    ShardedVectorType vector_type = CreateShardedVectorType(
        reduce->shape().element_type(),
        innermost_dimension_size % vectorization_factor);
    TF_ASSIGN_OR_RETURN(std::vector<llvm::Value*> accumulator,
                        EmitInnerLoopForVectorizedReduction(
                            reduction_generator, array_index, vector_type,
                            init_value, arg, dimensions, element_alignment));

    llvm_ir::IrArray target_array = GetIrArrayFor(reduce);
    llvm::Value* output_address =
        target_array.EmitArrayElementAddress(array_index, &b_);
    EmitShardedVectorStore(output_address, accumulator, element_alignment,
                           target_array);
  }

  if (outermost_loop_exit_block) {
    b_.SetInsertPoint(outermost_loop_exit_block);
  }

  return true;
}

StatusOr<llvm::Value*> IrEmitter::EmitTargetElementLoopBodyForReduce(
    HloReduceInstruction* reduce, const llvm_ir::IrArray::Index& index) {
  const HloInstruction* arg = reduce->mutable_operand(0);
  const HloInstruction* init_value = reduce->mutable_operand(1);
  absl::Span<const int64> dimensions(reduce->dimensions());

  // Initialize an accumulator with init_value.
  PrimitiveType accumulator_type = reduce->shape().element_type();
  llvm::AllocaInst* accumulator_addr = llvm_ir::EmitAllocaAtFunctionEntry(
      llvm_ir::PrimitiveTypeToIrType(accumulator_type, module_), "accumulator",
      &b_, MinimumAlignmentForPrimitiveType(accumulator_type));
  llvm::Value* init_value_addr = GetEmittedValueFor(init_value);
  llvm::Value* load_init_value = Load(init_value_addr);
  Store(load_init_value, accumulator_addr);

  // The enclosing loops go over all the target elements. Now we have to compute
  // the actual target element. For this, we build a new loop nest to iterate
  // over all the reduction dimensions in the argument.
  // AddLoopsForShapeOnDimensions will return an Index where induction Value*s
  // are placed for each dimension in dimensions, and all the rest are nullptrs.
  llvm_ir::ForLoopNest loops(IrName(reduce, "inner"), &b_);
  const llvm_ir::IrArray::Index reduced_dims_index =
      loops.AddLoopsForShapeOnDimensions(arg->shape(), dimensions,
                                         "reduction_dim");

  SetToFirstInsertPoint(loops.GetInnerLoopBodyBasicBlock(), &b_);

  // Build a full index for the input argument, using reduced_dims_index as the
  // base. In reduced_dims_index only the reduction dimensions are filled in. We
  // fill in the rest of the dimensions with induction Value*s taken from
  // 'index' which iterates over the target array.  See the high-level
  // description in the XLA documentation for details.
  llvm_ir::IrArray arg_array(GetIrArrayFor(arg));
  llvm_ir::IrArray::Index input_index = reduced_dims_index;
  llvm_ir::IrArray::Index::const_iterator it = index.begin();

  for (size_t i = 0; i < input_index.size(); ++i) {
    if (input_index[i] == nullptr) {
      input_index[i] = *it++;
    }
  }
  CHECK(index.end() == it);

  // Apply the reduction function to the loaded value.
  llvm::Value* input_element = arg_array.EmitReadArrayElement(input_index, &b_);
  llvm::Value* result = EmitThreadLocalCall(
      *reduce->to_apply(), {Load(accumulator_addr), input_element},
      "reduce_function");
  Store(result, accumulator_addr);

  SetToFirstInsertPoint(loops.GetOuterLoopExitBasicBlock(), &b_);
  return Load(accumulator_addr);
}

Status IrEmitter::HandleReduce(HloInstruction* reduce) {
  // TODO(b/112040122): Support variadic reduce.
  if (!ShapeUtil::IsArray(reduce->shape())) {
    return Unimplemented("Variadic reduce is not supported on CPU");
  }
  auto arg = reduce->mutable_operand(0);
  auto init_value = reduce->mutable_operand(1);
  absl::Span<const int64> dimensions(reduce->dimensions());
  HloComputation* function = reduce->to_apply();
  if (!options::VectorizedReduceDisabled(hlo_module_config_)) {
    string vectorization_failure_reason;
    TF_ASSIGN_OR_RETURN(
        bool vectorization_successful,
        EmitVectorizedReduce(reduce, arg, init_value, dimensions, function,
                             &vectorization_failure_reason));
    if (vectorization_successful) {
      VLOG(1) << "Successfully vectorized reduction " << reduce->ToString()
              << "\n";
      return Status::OK();
    } else {
      VLOG(1) << "Could not vectorize reduction " << reduce->ToString() << ": "
              << vectorization_failure_reason;
    }
  }

  return EmitTargetElementLoop(reduce,
                               [&](const llvm_ir::IrArray::Index& index) {
                                 return EmitTargetElementLoopBodyForReduce(
                                     Cast<HloReduceInstruction>(reduce), index);
                               });
}

Status IrEmitter::HandleSend(HloInstruction* send) {
  // TODO(b/33942983): Support Send/Recv on CPU.
  return Unimplemented("Send is not implemented on CPU.");
}

Status IrEmitter::HandleSendDone(HloInstruction* send_done) {
  // TODO(b/33942983): Support Send/Recv on CPU.
  return Unimplemented("Send-done is not implemented on CPU.");
}

Status IrEmitter::HandleScatter(HloInstruction*) {
  return Unimplemented("Scatter is not implemented on CPUs.");
}

Status IrEmitter::HandleSlice(HloInstruction* slice) {
  VLOG(2) << "HandleSlice: " << slice->ToString();
  auto operand = slice->operand(0);
  // The code below emits a sequential loop nest. For the parallel backend, use
  // ParallelLoopEmitter which respects dynamic loop bounds.
  if (ShouldEmitParallelLoopFor(*slice)) {
    return DefaultAction(slice);
  }

  // The code below assumes the layouts are equal.
  if (!LayoutUtil::Equal(operand->shape().layout(), slice->shape().layout())) {
    return DefaultAction(slice);
  }

  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(slice));

  if (ShapeUtil::IsZeroElementArray(slice->shape())) {
    return Status::OK();
  }

  const Layout& layout = operand->shape().layout();
  const int64 num_dims = operand->shape().dimensions_size();

  // The slice lowering finds maximal contiguous blocks of memory that can be
  // copied from the source to the target. This is done by looking at the
  // source/target layout in minor to major order and do the following:
  //
  // * Find an initial segment of dimensions along which the slice uses the
  //   whole dimension. These are the "inner" dimensions and can be folded into
  //   the memcpy.
  //
  // * Of the remaining dimensions decide which ones require loops.
  //
  // * Implement the memcpy within the innermost loop.

  absl::flat_hash_set<int64> inner_dims;
  for (int64 dim : LayoutUtil::MinorToMajor(layout)) {
    if (operand->shape().dimensions(dim) != slice->shape().dimensions(dim)) {
      break;
    }
    inner_dims.insert(dim);
  }

  const bool is_trivial_copy = (inner_dims.size() == num_dims);
  if (is_trivial_copy) {
    if (ShapeUtil::IsEffectiveScalar(slice->shape())) {
      return DefaultAction(slice);
    } else {
      return EmitMemcpy(*slice, *operand);
    }
  }

  // The memcpy will copy elements that are logically this shape (allowed to be
  // scalar).
  const Shape logical_element_shape = ShapeUtil::FilterDimensions(
      [&inner_dims](int64 dim) -> bool { return inner_dims.count(dim); },
      operand->shape());

  const int64 primitive_elements_per_logical_element =
      ShapeUtil::ElementsIn(logical_element_shape);

  // memcpy_dim is the innermost (in terms of layout) dimension for which the
  // slice does *not* just copy all the elements along the dimension.
  const int64 memcpy_dim = LayoutUtil::Minor(layout, inner_dims.size());

  const bool memcpy_is_contiguous = slice->slice_strides(memcpy_dim) == 1;
  // The number of logical elements that can be copied in a single call
  // to memcpy. We can only copy 1 element at a time if there is a non-trivial
  // stride.
  const int64 memcpy_logical_elements =
      memcpy_is_contiguous
          ? slice->slice_limits(memcpy_dim) - slice->slice_starts(memcpy_dim)
          : 1;

  // Determine the dimensions that get lowered as loops.
  std::vector<int64> outer_dims;
  for (int64 i = 0; i < num_dims - inner_dims.size() - 1; ++i) {
    outer_dims.push_back(LayoutUtil::Major(layout, i));
  }

  // Is the slice along the memcpy dimension contiguous? If not, then memcpy_dim
  // needs to be wrapped around a loop as well.
  if (!memcpy_is_contiguous) {
    outer_dims.push_back(memcpy_dim);
  }

  llvm_ir::IrArray target_array = GetIrArrayFor(slice);

  const int64 num_outer_loops = outer_dims.size();
  llvm_ir::ForLoopNest loops(IrName(slice), &b_);
  llvm_ir::IrArray::Index target_index =
      loops.AddLoopsForShapeOnDimensions(slice->shape(), outer_dims, "slice");

  // Only the indices for the outer dimensions have been initialized in
  // target_index. The rest of the indices should get initialized to 0, since
  // for the rest of the dimensions the copy writes to the full dimension.
  std::replace(target_index.begin(), target_index.end(),
               static_cast<llvm::Value*>(nullptr),
               static_cast<llvm::Value*>(b_.getInt64(0)));

  if (num_outer_loops > 0) {
    SetToFirstInsertPoint(loops.GetInnerLoopBodyBasicBlock(), &b_);
  }

  llvm_ir::IrArray source_array = GetIrArrayFor(operand);
  const llvm_ir::IrArray::Index source_index = target_index.SourceIndexOfSlice(
      /*shape=*/slice->shape(), /*starts=*/slice->slice_starts(),
      /*strides=*/slice->slice_strides(), /*builder=*/&b_);

  llvm::Value* memcpy_dest =
      target_array.EmitArrayElementAddress(target_index, &b_, "slice.dest");
  llvm::Value* memcpy_source =
      source_array.EmitArrayElementAddress(source_index, &b_, "slice.source");

  const int64 memcpy_elements =
      primitive_elements_per_logical_element * memcpy_logical_elements;

  EmitTransferElements(memcpy_dest, memcpy_source, memcpy_elements,
                       slice->shape().element_type(), target_array,
                       source_array);

  if (VLOG_IS_ON(2)) {
    const int64 memcpy_bytes =
        ShapeUtil::ByteSizeOf(logical_element_shape) * memcpy_elements;
    VLOG(2) << "  emitted copy of " << memcpy_bytes << " bytes inside "
            << num_outer_loops << " loops";
  }

  if (num_outer_loops > 0) {
    SetToFirstInsertPoint(loops.GetOuterLoopExitBasicBlock(), &b_);
  }

  return Status::OK();
}

Status IrEmitter::HandleDynamicSlice(HloInstruction* dynamic_slice) {
  if (ShapeUtil::IsScalar(dynamic_slice->shape())) {
    TF_RETURN_IF_ERROR(EmitTargetAddressForOp(dynamic_slice));
    return EmitMemcpy(*dynamic_slice->operand(0), *dynamic_slice);
  }
  return DefaultAction(dynamic_slice);
}

Status IrEmitter::HandleDynamicUpdateSlice(
    HloInstruction* dynamic_update_slice) {
  auto update = dynamic_update_slice->operand(1);
  if (ShapeUtil::IsScalar(dynamic_update_slice->shape())) {
    TF_RETURN_IF_ERROR(EmitTargetAddressForOp(dynamic_update_slice));
    return EmitMemcpy(*update, *dynamic_update_slice);
  } else if (llvm_ir::CanUpdateDynamicSliceInPlace(dynamic_update_slice,
                                                   assignment_)) {
    TF_RETURN_IF_ERROR(EmitTargetAddressForOp(dynamic_update_slice));
    auto operands = GetIrArraysForOperandsOf(dynamic_update_slice);
    return llvm_ir::EmitDynamicUpdateSliceInPlace(
        operands, GetIrArrayFor(dynamic_update_slice),
        IrName(dynamic_update_slice, "in_place"), &b_);
  }
  return DefaultAction(dynamic_update_slice);
}

Status IrEmitter::HandleRecv(HloInstruction* recv) {
  // TODO(b/33942983): Support Send/Recv on CPU.
  return Unimplemented("Recv is not implemented on CPU.");
}

Status IrEmitter::HandleRecvDone(HloInstruction* recv_done) {
  // TODO(b/33942983): Support Send/Recv on CPU.
  return Unimplemented("Recv-done is not implemented on CPU.");
}

Status IrEmitter::HandlePad(HloInstruction* pad) {
  // CPU backend does not properly handle negative padding but this is ok
  // because negative padding should be removed by the algebraic simplifier.
  for (auto& padding_dimension : pad->padding_config().dimensions()) {
    if (padding_dimension.edge_padding_low() < 0 ||
        padding_dimension.edge_padding_high() < 0) {
      return InternalErrorStrCat(
          "Encountered negative padding in IrEmitter on CPU. "
          "This should have been eliminated at the HLO level. ",
          pad->ToString());
    }
  }

  // First, fill in the padding value to all output elements.
  TF_RETURN_IF_ERROR(EmitTargetElementLoop(
      pad, "initialize",
      [this, pad](const llvm_ir::IrArray::Index& target_index) {
        const HloInstruction* padding_value = pad->operand(1);
        llvm::Value* padding_value_addr = GetEmittedValueFor(padding_value);
        return Load(padding_value_addr);
      }));

  // Create a loop to iterate over the operand elements and update the output
  // locations where the operand elements should be stored.
  llvm_ir::ForLoopNest loops(IrName(pad, "assign"), &b_);
  const HloInstruction* operand = pad->operand(0);
  const llvm_ir::IrArray::Index operand_index =
      loops.AddLoopsForShape(operand->shape(), "operand");

  SetToFirstInsertPoint(loops.GetInnerLoopBodyBasicBlock(), &b_);

  // Load an element from the operand.
  llvm_ir::IrArray operand_array(GetIrArrayFor(operand));
  llvm::Value* operand_data =
      operand_array.EmitReadArrayElement(operand_index, &b_);

  // Compute the output index the operand element should be assigned to.
  // output_index := edge_padding_low + operand_index * (interior_padding + 1)
  const PaddingConfig& padding_config = pad->padding_config();
  llvm_ir::IrArray::Index output_index(operand_index.GetType());
  for (size_t i = 0; i < operand_index.size(); ++i) {
    llvm::Value* offset =
        Mul(operand_index[i],
            b_.getInt64(padding_config.dimensions(i).interior_padding() + 1));
    llvm::Value* index = Add(
        offset, b_.getInt64(padding_config.dimensions(i).edge_padding_low()));
    output_index.push_back(index);
  }

  // Store the operand element to the computed output location.
  llvm_ir::IrArray output_array(GetIrArrayFor(pad));
  output_array.EmitWriteArrayElement(output_index, operand_data, &b_);

  SetToFirstInsertPoint(loops.GetOuterLoopExitBasicBlock(), &b_);
  return Status::OK();
}

Status IrEmitter::HandleFusion(HloInstruction* fusion) {
  auto* root = fusion->fused_expression_root();
  if (llvm_ir::CanEmitFusedDynamicUpdateSliceInPlace(fusion, assignment_)) {
    VLOG(3) << "HandleFusion FusedDynamicUpdateSliceInPlace";
    CpuElementalIrEmitter elemental_emitter(hlo_module_config_, this, module_);
    TF_RETURN_IF_ERROR(EmitTargetAddressForOp(fusion));
    // Delegate to common implementation of fused in-place dynamic-update-slice.
    return llvm_ir::EmitFusedDynamicUpdateSliceInPlace(
        fusion, GetGeneratorForOperandIrArrays(fusion), GetIrArrayFor(fusion),
        &elemental_emitter, &b_);
  } else if (fusion->fusion_kind() == HloInstruction::FusionKind::kLoop) {
    VLOG(3) << "HandleFusion kLoop";
    CpuElementalIrEmitter elemental_emitter(hlo_module_config_, this, module_);
    auto operands = GetIrArraysForOperandsOf(fusion);
    FusedIrEmitter fused_emitter(GetGeneratorForOperandIrArrays(fusion),
                                 &elemental_emitter);
    TF_RETURN_IF_ERROR(fusion->fused_expression_root()->Accept(&fused_emitter));

    return EmitTargetElementLoop(fusion, fused_emitter.GetRootGenerator());
  } else if (fusion->fusion_kind() == HloInstruction::FusionKind::kOutput) {
    VLOG(3) << "HandleFusion kOutput";
    int64 dot_op_index = root->operand(0)->opcode() == HloOpcode::kDot ? 0 : 1;
    const HloInstruction* dot = root->operand(dot_op_index);
    CHECK_EQ(dot->opcode(), HloOpcode::kDot)
        << dot->ToString() << "  "
        << fusion->fused_instructions_computation()->ToString();

    int64 dot_lhs_param_number = dot->operand(0)->parameter_number();
    int64 dot_rhs_param_number = dot->operand(1)->parameter_number();
    int64 addend_param_number =
        root->operand(1 - dot_op_index)->parameter_number();

    Shape target_shape = fusion->shape();
    TF_RETURN_IF_ERROR(EmitTargetAddressForOp(fusion));
    llvm_ir::IrArray target_array = GetIrArrayFor(fusion);

    llvm_ir::IrArray lhs_array(
        GetIrArrayFor(fusion->operand(dot_lhs_param_number)));
    llvm_ir::IrArray rhs_array(
        GetIrArrayFor(fusion->operand(dot_rhs_param_number)));
    llvm_ir::IrArray addend_array(
        GetIrArrayFor(fusion->operand(addend_param_number)));

    TF_RETURN_IF_ERROR(DotOpEmitter::EmitDotOperation(
        *dot, target_array, lhs_array, rhs_array, &addend_array,
        GetExecutableRunOptionsArgument(), &b_, hlo_module_config_,
        target_machine_features_));
    return Status::OK();
  } else {
    return Unimplemented("Fusion kind not implemented on CPU");
  }
}

Status IrEmitter::HandleCall(HloInstruction* call) {
  HloComputation* computation = call->to_apply();
  llvm::Function* call_ir_function = FindOrDie(emitted_functions_, computation);

  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(call));

  if (!computation->root_instruction()->outer_dimension_partitions().empty()) {
    // ParallelTaskAssignment assigned partitions, emit call to
    // ParallelForkJoin.
    std::vector<llvm::Value*> call_args = GetArrayFunctionCallArguments(
        {}, &b_, computation->name(),
        /*return_value_buffer=*/emitted_value_[call],
        /*exec_run_options_arg=*/GetExecutableRunOptionsArgument(),
        /*buffer_table_arg=*/GetBufferTableArgument(),
        /*profile_counters_arg=*/GetProfileCountersArgument());

    HloInstruction* root = computation->root_instruction();
    TF_RETURN_IF_ERROR(EmitCallToParallelForkJoin(
        call_args, root->shape(), root->outer_dimension_partitions(), &b_,
        call_ir_function, computation->name()));
  } else {
    EmitGlobalCall(*computation, computation->name());
  }

  return Status::OK();
}

Status IrEmitter::HandleCustomCall(HloInstruction* custom_call) {
  absl::Span<HloInstruction* const> operands(custom_call->operands());
  absl::string_view custom_call_target(custom_call->custom_call_target());
  llvm::Type* i8_ptr_type = b_.getInt8PtrTy();
  llvm::AllocaInst* operands_alloca =
      llvm_ir::EmitAllocaAtFunctionEntryWithCount(
          i8_ptr_type, b_.getInt32(operands.size()), "cc_operands_alloca", &b_);
  for (size_t i = 0; i < operands.size(); ++i) {
    const HloInstruction* operand = operands[i];
    llvm::Value* operand_as_i8ptr =
        PointerCast(GetEmittedValueFor(operand), i8_ptr_type);
    llvm::Value* slot_in_operands_alloca =
        InBoundsGEP(operands_alloca, {b_.getInt64(i)});
    Store(operand_as_i8ptr, slot_in_operands_alloca);
  }
  auto* custom_call_ir_function =
      llvm::cast<llvm::Function>(module_->getOrInsertFunction(
          AsStringRef(custom_call_target),
          llvm::FunctionType::get(
              /*Result=*/b_.getVoidTy(),
              /*Params=*/{i8_ptr_type, operands_alloca->getType()},
              /*isVarArg=*/false)));

  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(custom_call));
  // Write the tuple table if the output is a tuple.
  if (ShapeUtil::IsTuple(custom_call->shape())) {
    std::vector<llvm::Value*> base_ptrs;
    for (int i = 0; i < ShapeUtil::TupleElementCount(custom_call->shape());
         ++i) {
      const Shape& elem_shape =
          ShapeUtil::GetTupleElementShape(custom_call->shape(), i);
      TF_RET_CHECK(!ShapeUtil::IsTuple(elem_shape))
          << "Nested tuples not implemented";
      TF_ASSIGN_OR_RETURN(const BufferAllocation::Slice slice,
                          assignment_.GetUniqueSlice(custom_call, {i}));
      llvm::Value* addr = EmitBufferPointer(slice, elem_shape);
      base_ptrs.push_back(addr);
    }
    llvm_ir::EmitTuple(GetIrArrayFor(custom_call), base_ptrs, &b_, module_);
  }
  auto* output_address_arg =
      PointerCast(GetEmittedValueFor(custom_call), i8_ptr_type);

  Call(custom_call_ir_function, {output_address_arg, operands_alloca});

  return Status::OK();
}

Status IrEmitter::HandleWhile(HloInstruction* xla_while) {
  // Precondition: Condition computation must return a scalar bool.
  HloComputation* condition = xla_while->while_condition();
  TF_RET_CHECK(ShapeUtil::IsScalar(condition->root_instruction()->shape()) &&
               condition->root_instruction()->shape().element_type() == PRED)
      << "While condition computation must return bool; got: "
      << ShapeUtil::HumanString(condition->root_instruction()->shape());
  // Check that all while-related buffers share an allocation slice.
  TF_RETURN_IF_ERROR(ShapeUtil::ForEachSubshapeWithStatus(
      xla_while->shape(),
      [this, &xla_while](const Shape& /*subshape*/,
                         const ShapeIndex& index) -> Status {
        auto check = [this](const HloInstruction* a, const HloInstruction* b,
                            const ShapeIndex& index) {
          const BufferAllocation::Slice slice_a =
              assignment_.GetUniqueSlice(a, index).ConsumeValueOrDie();
          const BufferAllocation::Slice slice_b =
              assignment_.GetUniqueSlice(b, index).ConsumeValueOrDie();
          if (slice_a != slice_b) {
            return InternalError(
                "instruction %s %s does not share slice with "
                "instruction %s %s",
                a->ToString(), slice_a.ToString(), b->ToString(),
                slice_b.ToString());
          }
          return Status::OK();
        };
        TF_RETURN_IF_ERROR(check(xla_while, xla_while->operand(0), index));
        TF_RETURN_IF_ERROR(check(
            xla_while, xla_while->while_condition()->parameter_instruction(0),
            index));
        TF_RETURN_IF_ERROR(
            check(xla_while, xla_while->while_body()->parameter_instruction(0),
                  index));
        TF_RETURN_IF_ERROR(check(
            xla_while, xla_while->while_body()->root_instruction(), index));
        return Status::OK();
      }));

  // Set emitted value to that of 'init' with which it shares an allocation.
  const HloInstruction* init = xla_while->operand(0);
  emitted_value_[xla_while] = GetEmittedValueFor(init);

  // Generating:
  //   while (Condition(while_result)) {
  //     // CopyInsertion pass inserts copies which enable 'while_result' to
  //     // be passed back in as 'Body' parameter.
  //     while_result = Body(while_result);  // Insert
  //   }

  // Terminates the current block with a branch to a while header.
  llvm::BasicBlock* header_bb = llvm::BasicBlock::Create(
      module_->getContext(), AsStringRef(IrName(xla_while, "header")),
      compute_function_->function());
  Br(header_bb);
  b_.SetInsertPoint(header_bb);

  // Calls the condition function to determine whether to proceed with the
  // body.  It must return a bool, so use the scalar call form.
  EmitGlobalCall(*xla_while->while_condition(), IrName(xla_while, "cond"));
  llvm::Value* while_predicate = ICmpNE(
      Load(GetBufferForGlobalCallReturnValue(*xla_while->while_condition())),
      llvm::ConstantInt::get(llvm_ir::PrimitiveTypeToIrType(PRED, module_), 0));

  // Branches to the body or to the while exit depending on the condition.
  llvm::BasicBlock* body_bb = llvm::BasicBlock::Create(
      module_->getContext(), AsStringRef(IrName(xla_while, "body")),
      compute_function_->function());
  llvm::BasicBlock* exit_bb = llvm::BasicBlock::Create(
      module_->getContext(), AsStringRef(IrName(xla_while, "exit")));
  CondBr(while_predicate, body_bb, exit_bb);

  // Calls the body function from the body block.
  b_.SetInsertPoint(body_bb);

  // Calls the body function.
  EmitGlobalCall(*xla_while->while_body(), IrName(xla_while, "body"));

  // Finishes with a branch back to the header.
  Br(header_bb);

  // Adds the exit block to the function and sets the insert point there.
  compute_function_->function()->getBasicBlockList().push_back(exit_bb);
  b_.SetInsertPoint(exit_bb);

  return Status::OK();
}

StatusOr<bool> IrEmitter::EmitFastConcatenate(
    HloInstruction* concatenate, absl::Span<HloInstruction* const> operands,
    string* failure_reason) {
  if (ShouldEmitParallelLoopFor(*concatenate)) {
    *failure_reason =
        "cannot generate memcpy-based concat for the parallel CPU backend";
    return false;
  }

  const Shape& output_shape = concatenate->shape();
  for (auto* op : operands) {
    if (!LayoutUtil::Equal(op->shape().layout(), output_shape.layout())) {
      *failure_reason = "operand has mismatching layouts";
      return false;
    }
  }

  // We split the dimensions into three categories: the dimension over which we
  // are concatenating (concat_dim), the dimensions that are minor to it
  // (inner_dims) and the dimensions that are major to it (outer_dims).

  int64 concat_dim = concatenate->dimensions(0);
  const Layout& output_layout = output_shape.layout();
  auto output_min2maj = LayoutUtil::MinorToMajor(output_layout);
  auto concat_dim_layout_itr =
      std::find(output_min2maj.begin(), output_min2maj.end(), concat_dim);

  std::vector<int64> inner_dims(output_min2maj.begin(), concat_dim_layout_itr);
  std::vector<int64> outer_dims(std::next(concat_dim_layout_itr),
                                output_min2maj.end());

  llvm::Type* i8_ptr_type = b_.getInt8PtrTy();

  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(concatenate));
  llvm_ir::IrArray target_array = GetIrArrayFor(concatenate);

  llvm_ir::ForLoopNest loops(IrName(concatenate), &b_);
  llvm_ir::IrArray::Index outer_dims_index =
      loops.AddLoopsForShapeOnDimensions(output_shape, outer_dims, "concat");
  std::replace(outer_dims_index.begin(), outer_dims_index.end(),
               static_cast<llvm::Value*>(nullptr),
               static_cast<llvm::Value*>(b_.getInt64(0)));

  if (!outer_dims.empty()) {
    SetToFirstInsertPoint(loops.GetInnerLoopBodyBasicBlock(), &b_);
  }

  PrimitiveType primitive_type = output_shape.element_type();
  unsigned primitive_type_size =
      ShapeUtil::ByteSizeOfPrimitiveType(primitive_type);

  // Contiguous subregions from each operand to the concatenate contribute to a
  // contiguous subregion in the target buffer starting at target_region_begin.
  llvm::Value* target_region_begin =
      BitCast(target_array.EmitArrayElementAddress(outer_dims_index, &b_,
                                                   "target_region"),
              i8_ptr_type);
  int64 byte_offset_into_target_region = 0;

  int64 inner_dims_product =
      std::accumulate(inner_dims.begin(), inner_dims.end(), 1l,
                      [&](int64 product, int64 inner_dim) {
                        return product * output_shape.dimensions(inner_dim);
                      });

  // For each operand, emit a memcpy from the operand to the target of size
  // equal to the product of inner dimensions.
  for (HloInstruction* operand : operands) {
    const Shape& input_shape = operand->shape();
    llvm_ir::IrArray source_array = GetIrArrayFor(operand);
    llvm::Value* copy_source_address = BitCast(
        source_array.EmitArrayElementAddress(outer_dims_index, &b_, "src_addr"),
        i8_ptr_type);

    llvm::Value* copy_target_address =
        GEP(target_region_begin, b_.getInt64(byte_offset_into_target_region));

    EmitTransferElements(
        copy_target_address, copy_source_address,
        inner_dims_product * input_shape.dimensions(concat_dim), primitive_type,
        target_array, source_array);

    byte_offset_into_target_region += inner_dims_product *
                                      input_shape.dimensions(concat_dim) *
                                      primitive_type_size;
  }

  if (!outer_dims.empty()) {
    SetToFirstInsertPoint(loops.GetOuterLoopExitBasicBlock(), &b_);
  }

  return true;
}

void IrEmitter::EmitTransferElements(llvm::Value* target, llvm::Value* source,
                                     int64 element_count,
                                     PrimitiveType primitive_type,
                                     const llvm_ir::IrArray& target_array,
                                     const llvm_ir::IrArray& source_array) {
  unsigned primitive_type_size =
      ShapeUtil::ByteSizeOfPrimitiveType(primitive_type);
  unsigned element_alignment = tensorflow::MathUtil::GCD<unsigned>(
      primitive_type_size, MinimumAlignmentForPrimitiveType(primitive_type));
  llvm::Type* primitive_ptr_type = llvm::PointerType::getUnqual(
      llvm_ir::PrimitiveTypeToIrType(primitive_type, module_));

  if (element_count == 1) {
    auto* load_instruction =
        AlignedLoad(BitCast(source, primitive_ptr_type), element_alignment);
    source_array.AnnotateLoadStoreInstructionWithMetadata(load_instruction);
    auto* store_instruction =
        AlignedStore(load_instruction, BitCast(target, primitive_ptr_type),
                     element_alignment);
    target_array.AnnotateLoadStoreInstructionWithMetadata(store_instruction);
  } else {
    auto* memcpy_instruction = MemCpy(
        target, /*DstAlign=*/element_alignment, source,
        /*SrcAlign=*/element_alignment, element_count * primitive_type_size);

    // The memcpy does the load and the store internally.  The aliasing related
    // metadata has to reflect that.
    std::map<int, llvm::MDNode*> merged_metadata =
        llvm_ir::MergeMetadata(&module_->getContext(), source_array.metadata(),
                               target_array.metadata());
    for (const auto& kind_md_pair : merged_metadata) {
      memcpy_instruction->setMetadata(kind_md_pair.first, kind_md_pair.second);
    }
  }
}

Status IrEmitter::HandleConcatenate(HloInstruction* concatenate) {
  absl::Span<HloInstruction* const> operands(concatenate->operands());
  string failure_reason;
  TF_ASSIGN_OR_RETURN(
      bool successful,
      EmitFastConcatenate(concatenate, operands, &failure_reason));
  if (successful) {
    VLOG(1) << "Emitted fast concatenate for " << concatenate->ToString();
    return Status::OK();
  }

  VLOG(1) << "Could not emit fast concatenate for " << concatenate->ToString()
          << ": " << failure_reason;

  return DefaultAction(concatenate);
}

Status IrEmitter::HandleConditional(HloInstruction* conditional) {
  auto pred = conditional->operand(0);
  TF_RET_CHECK(ShapeUtil::IsScalar(pred->shape()) &&
               pred->shape().element_type() == PRED)
      << "Predicate on a Conditional must be bool; got: "
      << ShapeUtil::HumanString(pred->shape());

  HloComputation* true_computation = conditional->true_computation();
  HloComputation* false_computation = conditional->false_computation();
  TF_RET_CHECK(ShapeUtil::Equal(conditional->shape(),
                                true_computation->root_instruction()->shape()))
      << "Shape of conditional should be same as the shape of the true "
      << "computation; got: " << ShapeUtil::HumanString(conditional->shape())
      << " and "
      << ShapeUtil::HumanString(true_computation->root_instruction()->shape());

  TF_RET_CHECK(ShapeUtil::Equal(conditional->shape(),
                                false_computation->root_instruction()->shape()))
      << "Shape of conditional should be same as the shape of the false "
      << "computation; got: " << ShapeUtil::HumanString(conditional->shape())
      << " and "
      << ShapeUtil::HumanString(false_computation->root_instruction()->shape());

  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(conditional));

  // Generating:
  //   if (pred)
  //     cond_result = true_computation(true_operand)
  //   else
  //     cond_result = false_computation(false_operand)
  llvm::LoadInst* pred_value =
      Load(GetIrArrayFor(pred).GetBasePointer(), "load_predicate_value");
  llvm::Value* pred_cond = ICmpNE(
      pred_value,
      llvm::ConstantInt::get(llvm_ir::PrimitiveTypeToIrType(PRED, module_), 0),
      "boolean_predicate");
  llvm_ir::LlvmIfData if_data =
      llvm_ir::EmitIfThenElse(pred_cond, "conditional", &b_);

  SetToFirstInsertPoint(if_data.true_block, &b_);
  EmitGlobalCall(*conditional->true_computation(),
                 IrName(conditional, "_true"));

  SetToFirstInsertPoint(if_data.false_block, &b_);
  EmitGlobalCall(*conditional->false_computation(),
                 IrName(conditional, "_false"));

  SetToFirstInsertPoint(if_data.after_block, &b_);
  return Status::OK();
}

Status IrEmitter::HandleAfterAll(HloInstruction* after_all) {
  TF_RET_CHECK(ByteSizeOf(after_all->shape()) == 0);
  // No code to generate, but we need to emit an address for book-keeping.
  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(after_all));
  return Status::OK();
}

Status IrEmitter::HandleAddDependency(HloInstruction* add_dependency) {
  // AddDedendency just forwards its zero-th operand.
  emitted_value_[add_dependency] =
      GetEmittedValueFor(add_dependency->operand(0));
  return Status::OK();
}

Status IrEmitter::HandleRng(HloInstruction* rng) {
  ElementalIrEmitter::HloToElementGeneratorMap operand_to_generator;
  for (const HloInstruction* operand : rng->operands()) {
    operand_to_generator[operand] = [=](const llvm_ir::IrArray::Index& index) {
      return GetIrArrayFor(operand).EmitReadArrayElement(index, &b_);
    };
  }

  CpuElementalIrEmitter elemental_emitter(hlo_module_config_, this, module_);
  TF_RETURN_IF_ERROR(EmitTargetElementLoop(
      rng, elemental_emitter.MakeElementGenerator(rng, operand_to_generator)));

  llvm_ir::IncrementVariableForPhiloxRngState(1, module_, &b_);

  return Status::OK();
}

Status IrEmitter::FinishVisit(HloInstruction* root) {
  // When this method is called, we should have already emitted an IR value for
  // the root (return) op. The IR value holds the address of the buffer holding
  // the value. If the root is a constant or parameter, we perform a memcpy from
  // this buffer to the retval buffer of the computation. Otherwise, there's
  // nothing to do since the result was already written directly into the output
  // buffer.
  VLOG(2) << "FinishVisit root: " << root->ToString();
  if (root->opcode() == HloOpcode::kOutfeed) {
    VLOG(2) << "  outfeed with value: "
            << llvm_ir::DumpToString(*GetEmittedValueFor(root->operand(0)));
  } else {
    VLOG(2) << "  value: " << llvm_ir::DumpToString(*GetEmittedValueFor(root));
  }

  auto record_complete_computation = [&](llvm::Value* prof_counter) {
    if (prof_counter) {
      profiling_state_.RecordCompleteComputation(&b_, prof_counter);
    }
  };

  // For the entry computation this increment is cumulative of embedded
  // computations since it includes cycles spent in computations invoked by
  // While, Call etc.
  record_complete_computation(GetProfileCounterFor(*root->parent()));
  return Status::OK();
}

template <typename T>
llvm::Value* IrEmitter::GetProfileCounterCommon(
    const T& hlo,
    const std::unordered_map<const T*, int64>& profile_index_map) {
  auto it = profile_index_map.find(&hlo);
  if (it == profile_index_map.end()) {
    return nullptr;
  }

  int64 prof_counter_idx = it->second;
  string counter_name = IrName("prof_counter", hlo.name());
  return GEP(GetProfileCountersArgument(), b_.getInt64(prof_counter_idx),
             AsStringRef(counter_name));
}

void IrEmitter::ProfilingState::UpdateProfileCounter(llvm::IRBuilder<>* b,
                                                     llvm::Value* prof_counter,
                                                     llvm::Value* cycle_end,
                                                     llvm::Value* cycle_start) {
  auto* cycle_diff = b->CreateSub(cycle_end, cycle_start);
  llvm::LoadInst* old_cycle_count =
      b->CreateLoad(prof_counter, "old_cycle_count");
  auto* new_cycle_count =
      b->CreateAdd(cycle_diff, old_cycle_count, "new_cycle_count");
  b->CreateStore(new_cycle_count, prof_counter);
}

llvm::Value* IrEmitter::ProfilingState::ReadCycleCounter(llvm::IRBuilder<>* b) {
  llvm::Module* module = b->GetInsertBlock()->getModule();
  if (use_rdtscp_) {
    llvm::Function* func_llvm_readcyclecounter =
        llvm::Intrinsic::getDeclaration(module,
                                        llvm::Intrinsic::readcyclecounter);
    return b->CreateCall(func_llvm_readcyclecounter);
  }
  llvm::Function* func_llvm_x86_rdtscp =
      llvm::Intrinsic::getDeclaration(module, llvm::Intrinsic::x86_rdtscp);
  if (!aux_i8ptr_) {
    llvm::AllocaInst* rdtscp_aux =
        llvm_ir::EmitAllocaAtFunctionEntry(b->getInt32Ty(), "rdtscp_aux", b);
    aux_i8ptr_ = b->CreateBitCast(rdtscp_aux, b->getInt8PtrTy());
  }
  llvm::ConstantInt* alloca_size = b->getInt64(4);
  llvm::Function* func_llvm_lifetime_start =
      llvm::Intrinsic::getDeclaration(module, llvm::Intrinsic::lifetime_start);
  b->CreateCall(func_llvm_lifetime_start, {alloca_size, aux_i8ptr_});
  llvm::Value* rdtscp_call = b->CreateCall(func_llvm_x86_rdtscp, aux_i8ptr_);
  llvm::Function* func_llvm_lifetime_end =
      llvm::Intrinsic::getDeclaration(module, llvm::Intrinsic::lifetime_end);
  b->CreateCall(func_llvm_lifetime_end, {alloca_size, aux_i8ptr_});
  return rdtscp_call;
}

void IrEmitter::ProfilingState::RecordCycleStart(llvm::IRBuilder<>* b,
                                                 HloInstruction* hlo) {
  auto* cycle_start = ReadCycleCounter(b);
  cycle_start->setName(AsStringRef(IrName(hlo, "cycle_start")));
  cycle_starts_[hlo] = cycle_start;
  if (first_read_cycle_start_ == nullptr) {
    first_read_cycle_start_ = cycle_start;
  }
}

void IrEmitter::ProfilingState::RecordCycleDelta(llvm::IRBuilder<>* b,
                                                 HloInstruction* hlo,
                                                 llvm::Value* prof_counter) {
  auto* cycle_end = ReadCycleCounter(b);
  cycle_end->setName(AsStringRef(IrName(hlo, "cycle_end")));
  auto* cycle_start = cycle_starts_[hlo];
  UpdateProfileCounter(b, prof_counter, cycle_end, cycle_start);
  last_read_cycle_end_ = cycle_end;
}

void IrEmitter::ProfilingState::RecordCompleteComputation(
    llvm::IRBuilder<>* b, llvm::Value* prof_counter) {
  if (last_read_cycle_end_ && first_read_cycle_start_) {
    UpdateProfileCounter(b, prof_counter, last_read_cycle_end_,
                         first_read_cycle_start_);
  }
}

Status IrEmitter::Preprocess(HloInstruction* hlo) {
  VLOG(3) << "Visiting: " << hlo->ToString();
  if (instruction_to_profile_idx_.count(hlo)) {
    profiling_state_.RecordCycleStart(&b_, hlo);
  }
  return Status::OK();
}

Status IrEmitter::Postprocess(HloInstruction* hlo) {
  if (auto* prof_counter = GetProfileCounterFor(*hlo)) {
    profiling_state_.RecordCycleDelta(&b_, hlo, prof_counter);
  }
  return Status::OK();
}

llvm_ir::IrArray IrEmitter::GetIrArrayFor(const HloInstruction* hlo) {
  llvm::Value* value_for_op = GetEmittedValueFor(hlo);

  llvm_ir::IrArray array(value_for_op, hlo->shape());
  AddAliasingInformationToIrArray(*hlo, &array);
  return array;
}

std::vector<llvm_ir::IrArray> IrEmitter::GetIrArraysForOperandsOf(
    const HloInstruction* hlo) {
  std::vector<llvm_ir::IrArray> arrays;
  std::transform(
      hlo->operands().begin(), hlo->operands().end(),
      std::back_inserter(arrays),
      [&](const HloInstruction* operand) { return GetIrArrayFor(operand); });
  return arrays;
}

llvm::Value* IrEmitter::GetEmittedValueFor(const HloInstruction* hlo) {
  auto it = emitted_value_.find(hlo);
  if (it == emitted_value_.end()) {
    LOG(FATAL) << "could not find emitted value for: " << hlo->ToString();
  }
  return it->second;
}

llvm::Type* IrEmitter::IrShapeType(const Shape& shape) {
  return llvm_ir::ShapeToIrType(shape, module_);
}

llvm::Value* IrEmitter::GetProfileCountersArgument() {
  return compute_function_->profile_counters_arg();
}

llvm::Value* IrEmitter::GetBufferTableArgument() {
  return compute_function_->buffer_table_arg();
}

llvm::Value* IrEmitter::GetExecutableRunOptionsArgument() {
  return compute_function_->exec_run_options_arg();
}

llvm::Value* IrEmitter::EmitThreadLocalBufferPointer(
    const BufferAllocation::Slice& slice, const Shape& target_shape) {
  const BufferAllocation& allocation = *slice.allocation();
  llvm::Value* tempbuf_address = [&]() -> llvm::Value* {
    if (slice == computation_root_allocation_) {
      llvm::Argument* retval = compute_function_->result_arg();
      llvm::AttrBuilder attr_builder;
      attr_builder.addAlignmentAttr(MinimumAlignmentForShape(target_shape));
      attr_builder.addDereferenceableAttr(ByteSizeOf(target_shape));
      retval->addAttrs(attr_builder);
      return retval;
    }

    auto param_it =
        computation_parameter_allocations_.find(slice.allocation()->index());
    if (param_it != computation_parameter_allocations_.end()) {
      int64 param_number = param_it->second;
      // We have to access the parameter at offset param_number in the params
      // array. The code generated here is equivalent to this C code:
      //
      //   i8* param_address_untyped = params[param_number];
      //   Param* param_address_typed = (Param*)param_address_untyped;
      //
      // Where Param is the actual element type of the underlying buffer (for
      // example, float for an XLA F32 element type).
      llvm::Value* params = compute_function_->parameters_arg();
      llvm::Value* param_address_offset =
          llvm_ir::EmitBufferIndexingGEP(params, param_number, &b_);
      llvm::LoadInst* param_address_untyped = Load(param_address_offset);

      if (!ShapeUtil::IsOpaque(target_shape)) {
        AttachAlignmentMetadataForLoad(param_address_untyped, target_shape);
        AttachDereferenceableMetadataForLoad(param_address_untyped,
                                             target_shape);
      }
      return param_address_untyped;
    }

    // Thread-local allocations should only be assigned a single buffer.
    const auto& assigned_buffers = allocation.assigned_buffers();
    CHECK_EQ(1, assigned_buffers.size());
    const Shape& shape = assigned_buffers.begin()->first->shape();

    std::pair<llvm::Function*, BufferAllocation::Slice> key = {
        compute_function_->function(), slice};
    auto buf_it = thread_local_buffers_.find(key);
    if (buf_it == thread_local_buffers_.end()) {
      llvm::Value* buffer = llvm_ir::EmitAllocaAtFunctionEntry(
          IrShapeType(shape), absl::StrCat("thread_local", slice.ToString()),
          &b_, MinimumAlignmentForShape(target_shape));
      auto it_inserted_pair = thread_local_buffers_.insert({key, buffer});
      CHECK(it_inserted_pair.second);
      buf_it = it_inserted_pair.first;
    }
    return buf_it->second;
  }();
  return BitCast(tempbuf_address, IrShapeType(target_shape)->getPointerTo());
}

llvm::Value* IrEmitter::EmitGlobalBufferPointer(
    const BufferAllocation::Slice& slice, const Shape& target_shape) {
  const BufferAllocation& allocation = *slice.allocation();
  llvm::Value* tempbuf_address_ptr = llvm_ir::EmitBufferIndexingGEP(
      GetBufferTableArgument(), slice.index(), &b_);
  llvm::LoadInst* tempbuf_address_base = Load(tempbuf_address_ptr);
  if (hlo_module_config_.debug_options()
          .xla_llvm_enable_invariant_load_metadata()) {
    tempbuf_address_base->setMetadata(
        llvm::LLVMContext::MD_invariant_load,
        llvm::MDNode::get(tempbuf_address_base->getContext(), /*MDs=*/{}));
  }
  AttachAlignmentMetadataForLoad(tempbuf_address_base, allocation.size());
  AttachDereferenceableMetadataForLoad(tempbuf_address_base, allocation.size());

  llvm::Value* tempbuf_address_untyped = tempbuf_address_base;
  if (slice.offset() > 0) {
    // Adjust the address to account for the slice offset.
    tempbuf_address_untyped =
        InBoundsGEP(tempbuf_address_base, b_.getInt64(slice.offset()));
  }
  return BitCast(tempbuf_address_untyped,
                 IrShapeType(target_shape)->getPointerTo());
}

llvm::Value* IrEmitter::EmitBufferPointer(const BufferAllocation::Slice& slice,
                                          const Shape& target_shape) {
  if (slice.allocation()->is_thread_local()) {
    return EmitThreadLocalBufferPointer(slice, target_shape);
  } else if (slice.allocation()->is_constant()) {
    return FindOrDie(constant_buffer_to_global_, slice.allocation()->index());
  } else {
    return EmitGlobalBufferPointer(slice, target_shape);
  }
}

Status IrEmitter::EmitTargetAddressForOp(const HloInstruction* op) {
  const Shape& target_shape = op->shape();
  TF_ASSIGN_OR_RETURN(const BufferAllocation::Slice slice,
                      assignment_.GetUniqueTopLevelSlice(op));
  llvm::Value* addr = EmitBufferPointer(slice, target_shape);
  addr->setName(AsStringRef(IrName(op)));
  emitted_value_[op] = addr;
  return Status::OK();
}

Status IrEmitter::EmitTargetElementLoop(
    HloInstruction* target_op,
    const llvm_ir::ElementGenerator& element_generator) {
  return EmitTargetElementLoop(target_op, /*desc=*/"", element_generator);
}

Status IrEmitter::EmitTargetElementLoop(
    HloInstruction* target_op, absl::string_view desc,
    const llvm_ir::ElementGenerator& element_generator) {
  VLOG(2) << "EmitTargetElementLoop: " << target_op->ToString();

  const Shape& target_shape = target_op->shape();
  TF_RETURN_IF_ERROR(EmitTargetAddressForOp(target_op));
  llvm_ir::IrArray target_array = GetIrArrayFor(target_op);

  if (target_op->IsMultiOutputFusion()) {
    // For multiple outputs fusion, we need to emit each operand and the root.
    TF_RET_CHECK(num_dynamic_loop_bounds_ == 0);
    std::vector<llvm_ir::IrArray> output_arrays;
    for (int64 i = 0; i < ShapeUtil::TupleElementCount(target_shape); ++i) {
      TF_ASSIGN_OR_RETURN(BufferAllocation::Slice slice,
                          assignment_.GetUniqueSlice(target_op, {i}));
      const Shape& element_shape = ShapeUtil::GetSubshape(target_shape, {i});
      llvm::Value* op_target_address = EmitBufferPointer(slice, element_shape);
      output_arrays.push_back(
          llvm_ir::IrArray(op_target_address, element_shape));
    }
    TF_RETURN_IF_ERROR(
        llvm_ir::LoopEmitter(element_generator, output_arrays, &b_)
            .EmitLoop(IrName(target_op)));

    std::vector<llvm::Value*> tuple_operand_ptrs;
    for (int64 i = 0; i < output_arrays.size(); ++i) {
      tuple_operand_ptrs.push_back(output_arrays[i].GetBasePointer());
    }
    llvm_ir::EmitTuple(target_array, tuple_operand_ptrs, &b_, module_);

  } else {
    if (ShouldEmitParallelLoopFor(*target_op)) {
      // Emit code to read dynamic loop bounds from compute function argument.
      std::vector<std::pair<llvm::Value*, llvm::Value*>> dynamic_loop_bounds =
          compute_function_->GetDynamicLoopBounds();
      // Emit parallel loop with dynamic loop bounds for most-major dimensions.
      TF_RETURN_IF_ERROR(ParallelLoopEmitter(element_generator, target_array,
                                             &dynamic_loop_bounds, &b_)
                             .EmitLoop(IrName(target_op)));
    } else {
      TF_RETURN_IF_ERROR(
          llvm_ir::LoopEmitter(element_generator, target_array, &b_)
              .EmitLoop(IrName(target_op)));
    }
  }
  return Status::OK();
}

Status IrEmitter::EmitMemcpy(const HloInstruction& source,
                             const HloInstruction& destination) {
  llvm::Value* source_value = GetEmittedValueFor(&source);
  llvm::Value* destination_value = GetEmittedValueFor(&destination);
  int64 source_size = ByteSizeOf(source.shape());
  // TODO(b/63762267): Be more aggressive about specifying alignment.
  MemCpy(destination_value, /*DstAlign=*/1, source_value,
         /*SrcAlign=*/1, source_size);
  return Status::OK();
}

Status IrEmitter::ElementTypesSameAndSupported(
    const HloInstruction& instruction,
    absl::Span<const HloInstruction* const> operands,
    absl::Span<const PrimitiveType> supported_types) {
  for (auto operand : operands) {
    TF_RET_CHECK(
        ShapeUtil::SameElementType(operands[0]->shape(), operand->shape()));
  }

  TF_RET_CHECK(!operands.empty());
  PrimitiveType primitive_type = operands[0]->shape().element_type();
  if (std::find(supported_types.begin(), supported_types.end(),
                primitive_type) == supported_types.end()) {
    return Unimplemented("unsupported operand type %s in op %s",
                         PrimitiveType_Name(primitive_type),
                         HloOpcodeString(instruction.opcode()));
  }
  return Status::OK();
}

Status IrEmitter::DefaultAction(HloInstruction* hlo) {
  ElementalIrEmitter::HloToElementGeneratorMap operand_to_generator;
  for (const HloInstruction* operand : hlo->operands()) {
    operand_to_generator[operand] = [=](const llvm_ir::IrArray::Index& index) {
      return GetIrArrayFor(operand).EmitReadArrayElement(index, &b_);
    };
  }
  CpuElementalIrEmitter elemental_emitter(hlo_module_config_, this, module_);
  return EmitTargetElementLoop(
      hlo, elemental_emitter.MakeElementGenerator(hlo, operand_to_generator));
}

llvm::Value* IrEmitter::EmitThreadLocalCall(
    const HloComputation& callee, absl::Span<llvm::Value* const> parameters,
    absl::string_view name) {
  CHECK(absl::c_binary_search(thread_local_computations_, &callee));

  const Shape& return_shape = callee.root_instruction()->shape();

  // Lifting this restriction to allow "small" arrays should be easy.  Allowing
  // larger arrays is difficult because we allocate the buffer for this return
  // value on the stack.
  CHECK(ShapeUtil::IsScalar(return_shape));

  PrimitiveType return_type = return_shape.element_type();

  std::vector<llvm::Value*> parameter_addrs;
  for (llvm::Value* parameter : parameters) {
    CHECK(!parameter->getType()->isPointerTy());
    llvm::Value* parameter_addr = llvm_ir::EmitAllocaAtFunctionEntry(
        parameter->getType(), "arg_addr", &b_);
    Store(parameter, parameter_addr);
    parameter_addrs.push_back(parameter_addr);
  }

  llvm::Value* return_value_buffer = llvm_ir::EmitAllocaAtFunctionEntry(
      llvm_ir::PrimitiveTypeToIrType(return_type, module_),
      absl::StrCat(name, "_retval_addr"), &b_,
      MinimumAlignmentForPrimitiveType(return_type));

  Call(FindOrDie(emitted_functions_, &callee),
       GetArrayFunctionCallArguments(
           parameter_addrs, &b_, name,
           /*return_value_buffer=*/return_value_buffer,
           /*exec_run_options_arg=*/GetExecutableRunOptionsArgument(),
           /*buffer_table_arg=*/
           llvm::Constant::getNullValue(b_.getInt8PtrTy()->getPointerTo()),
           /*profile_counters_arg=*/GetProfileCountersArgument()));

  return Load(return_value_buffer);
}

void IrEmitter::EmitGlobalCall(const HloComputation& callee,
                               absl::string_view name) {
  CHECK(absl::c_binary_search(global_computations_, &callee));

  Call(FindOrDie(emitted_functions_, &callee),
       GetArrayFunctionCallArguments(
           /*parameter_addresses=*/{}, &b_, name,
           /*return_value_buffer=*/
           llvm::Constant::getNullValue(b_.getInt8PtrTy()),
           /*exec_run_options_arg=*/GetExecutableRunOptionsArgument(),
           /*buffer_table_arg=*/GetBufferTableArgument(),
           /*profile_counters_arg=*/GetProfileCountersArgument()));
}

llvm::Value* IrEmitter::GetBufferForGlobalCallReturnValue(
    const HloComputation& callee) {
  const HloInstruction* root_inst = callee.root_instruction();
  if (root_inst->opcode() == HloOpcode::kOutfeed) {
    return llvm::Constant::getNullValue(b_.getInt8PtrTy());
  }

  const BufferAllocation::Slice root_buffer =
      assignment_.GetUniqueTopLevelSlice(root_inst).ValueOrDie();
  return EmitBufferPointer(root_buffer, root_inst->shape());
}

}  // namespace cpu
}  // namespace xla
