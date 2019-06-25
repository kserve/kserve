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

#include "tensorflow/compiler/xla/service/heap_simulator.h"

#include <algorithm>
#include <vector>

#include "absl/container/flat_hash_map.h"
#include "absl/container/flat_hash_set.h"
#include "absl/memory/memory.h"
#include "tensorflow/compiler/xla/map_util.h"
#include "tensorflow/compiler/xla/util.h"

namespace xla {

using absl::flat_hash_map;
using absl::flat_hash_set;

/*static*/
StatusOr<int64> HeapSimulator::MinimumMemoryForModule(
    const HloSchedule& schedule,
    const LogicalBuffer::SizeFunction& size_function) {
  if (schedule.empty()) {
    return 0;
  }

  const HloModule* module = schedule.module();
  TF_ASSIGN_OR_RETURN(std::unique_ptr<TuplePointsToAnalysis> points_to_analysis,
                      TuplePointsToAnalysis::Run(module));

  // The absolute minimum memory required for a given sequence of instructions
  // is determined by the sequence of Alloc and Free calls on a simulated heap,
  // ignoring fragmentation. We run the heap simulation on the whole module,
  // rather than summing each computation, since it gives us a better lower
  // bound, by minimizing the liveness of sub-computations.
  TF_ASSIGN_OR_RETURN(
      HeapSimulator::Result result,
      HeapSimulator::Run(absl::make_unique<NoFragmentationStatsHeap>(), *module,
                         schedule, *points_to_analysis, size_function));
  return result.heap_size;
}

/*static*/
StatusOr<int64> HeapSimulator::MinimumMemoryForComputation(
    const HloComputation& computation, const HloInstructionSequence& sequence,
    const TuplePointsToAnalysis& points_to_analysis,
    const LogicalBuffer::SizeFunction& size_function,
    const absl::flat_hash_map<const HloComputation*, int64>*
        memory_by_computation) {
  TF_ASSIGN_OR_RETURN(
      HeapSimulator::Result result,
      HeapSimulator::Run(absl::make_unique<NoFragmentationStatsHeap>(),
                         computation, sequence, points_to_analysis,
                         size_function, HeapSimulator::Options(),
                         memory_by_computation));
  return result.heap_size;
}

/*static*/
StatusOr<HeapSimulator::Result> HeapSimulator::Run(
    std::unique_ptr<HeapAlgorithm> algorithm, const HloModule& module,
    const HloSchedule& schedule,
    const TuplePointsToAnalysis& points_to_analysis,
    const BufferValue::SizeFunction& size_fn, const Options& options) {
  HeapSimulator heap(std::move(algorithm), size_fn, options, &schedule);
  const HloComputation* entry_computation = module.entry_computation();
  const HloInstructionSequence& instruction_sequence =
      schedule.sequence(entry_computation);
  TF_RETURN_IF_ERROR(heap.RunComputation(
      *entry_computation, instruction_sequence, points_to_analysis));
  return heap.Finish();
}

/*static*/
StatusOr<HeapSimulator::Result> HeapSimulator::Run(
    std::unique_ptr<HeapAlgorithm> algorithm, const HloComputation& computation,
    const HloInstructionSequence& instruction_sequence,
    const TuplePointsToAnalysis& points_to_analysis,
    const BufferValue::SizeFunction& size_fn, const Options& options,
    const absl::flat_hash_map<const HloComputation*, int64>*
        memory_by_computation) {
  HeapSimulator heap(std::move(algorithm), size_fn, options,
                     /*schedule=*/nullptr, memory_by_computation);
  TF_RETURN_IF_ERROR(heap.RunComputation(computation, instruction_sequence,
                                         points_to_analysis));
  return heap.Finish();
}

// Runs a heap simulation for the given 'computation', assuming the given
// 'instruction_sequence'.
Status HeapSimulator::RunComputation(
    const HloComputation& computation,
    const HloInstructionSequence& instruction_sequence,
    const TuplePointsToAnalysis& points_to_analysis) {
  VLOG(3) << "Computation:\n" << computation.ToString();
  // The goal here is to minimize memory usage, assuming the given sequential
  // ordering of instructions.  The strategy is to walk through the instruction
  // sequence, calling Alloc and Free on the underlying heap algorithm.  The
  // heap algorithm takes care of packing and reducing fragmentation.
  //
  // 'live_buffers' tracks the liveness of each buffer that we assign, by
  // associating it with a set of HloInstructions that need to be visited.  When
  // the set becomes empty, the buffer is no longer used, and can be freed.
  // 'used_buffers' is the reverse map - it tracks which buffers were used by an
  // instruction, so that we can remove the instructions from a buffer's live
  // set after they are visited.
  flat_hash_map<const BufferValue*, flat_hash_set<const HloInstruction*>>
      live_buffers;
  flat_hash_map<const HloInstruction*, flat_hash_set<const BufferValue*>>
      used_buffers;
  auto add_user_to_buffer = [this, &live_buffers, &used_buffers](
                                const HloInstruction* user,
                                const BufferValue* buffer) {
    if (!IgnoreBuffer(buffer)) {
      VLOG(4) << "  Adding user " << user->name() << " to buffer "
              << buffer->ToString();
      live_buffers[buffer].insert(user);
      used_buffers[user].insert(buffer);
    }
  };

  // Initialize live_buffers for each buffer that we're going to assign.  The
  // set of instructions that need to be visited contains all users of all
  // aliases, that is, all users of all instructions that have the buffer
  // contained in their points-to set.
  for (const HloInstruction* instruction :
       instruction_sequence.instructions()) {
    const PointsToSet& points_to =
        points_to_analysis.GetPointsToSet(instruction);
    const PointsToSet::BufferSet& buffer_set = points_to.CreateFlattenedSet();
    for (const HloInstruction* user : instruction->users()) {
      if (user->opcode() != HloOpcode::kGetTupleElement) {
        for (const BufferValue* buffer : buffer_set) {
          add_user_to_buffer(user, buffer);
        }
      } else {
        // A GetTupleElement doesn't need to keep all of its operand's buffers
        // alive. It only needs the buffers that relate to the element it's
        // extracting, and the tuple it's extracting from, but not the buffers
        // for the other elements.
        for (const BufferValue* buffer : points_to.element({})) {
          add_user_to_buffer(user, buffer);
        }
        const PointsToSet& gte_points_to =
            points_to_analysis.GetPointsToSet(user);
        for (const BufferValue* buffer : gte_points_to.CreateFlattenedSet()) {
          add_user_to_buffer(user, buffer);
        }
      }
    }
  }

  const HloInstruction* root = computation.root_instruction();
  BufferValueCompactPointerSet output_source_buffers =
      ToBufferValueCompactPointerSet(
          points_to_analysis.GetPointsToSet(root).CreateFlattenedSet());

  std::vector<const BufferValue*> dead_buffers_to_free;
  std::vector<const BufferValue*> operand_buffers_to_free;
  for (const HloInstruction* instruction :
       instruction_sequence.instructions()) {
    const TuplePointsToAnalysis::BufferDefinitionVector&
        buffers_defined_by_instruction =
            points_to_analysis.GetBuffersDefinedByInstruction(instruction);

    VLOG(3) << "Instruction: " << instruction->ToString();
    for (const BufferValue* buffer : buffers_defined_by_instruction) {
      VLOG(4) << "  Defines: " << buffer->ToString()
              << (IgnoreBuffer(buffer) ? " (Ignored)" : "");
    }

    dead_buffers_to_free.clear();
    for (const BufferValue* buffer : buffers_defined_by_instruction) {
      if (IgnoreBuffer(buffer)) {
        continue;
      }
      // Add a nullptr sentry to ensure entry parameters and output source
      // buffers are not freed until the very end.
      const bool entry_parameter =
          &computation == computation.parent()->entry_computation() &&
          buffer->instruction()->opcode() == HloOpcode::kParameter;
      const bool output = output_source_buffers.count(buffer) > 0;
      if (entry_parameter || output) {
        live_buffers[buffer].insert(nullptr);
      }

      // If the buffer has no users and isn't an entry parameter or output, it
      // must be a dead value.
      if (live_buffers.count(buffer) == 0) {
        dead_buffers_to_free.push_back(buffer);
      }
    }

    // Update live_buffers to indicate we've visited this instruction; this is
    // the inverse of the initialization logic.  We erase this instruction from
    // all source buffers of all operands of this instruction.  Buffers that
    // have no instructions left to visit are moved from live_buffers to
    // operand_buffers_to_free.
    operand_buffers_to_free.clear();
    for (const BufferValue* operand_buffer : used_buffers[instruction]) {
      if (IgnoreBuffer(operand_buffer)) {
        continue;
      }
      VLOG(4) << "  Removing user " << instruction->name() << " from buffer "
              << operand_buffer->ToString();
      auto it = live_buffers.find(operand_buffer);
      flat_hash_set<const HloInstruction*>* live_set = &it->second;
      live_set->erase(instruction);
      if (live_set->empty()) {
        live_buffers.erase(it);
        operand_buffers_to_free.push_back(operand_buffer);
      }
    }
    // Sort to get a deterministic iteration order.
    std::sort(operand_buffers_to_free.begin(), operand_buffers_to_free.end(),
              [](const BufferValue* x, const BufferValue* y) {
                return x->id() < y->id();
              });

    // Allocate buffers defined by this instruction.  This is the latest point
    // that we can allocate; right before the buffer is first used.  This must
    // happen before dead or operand buffers are freed; the instruction reads
    // the operand buffers to produce its output.
    //
    // INVARIANT: Either Alloc or ShareBuffer will be called for each buffer
    // that we should assign.

    // Make sure each buffer get reused at most once.
    flat_hash_set<const BufferValue*> reused_buffers;
    int64 alloc_size_by_instruction = 0;
    for (const BufferValue* buffer : buffers_defined_by_instruction) {
      if (IgnoreBuffer(buffer)) {
        continue;
      }

      // Check whether the buffer can share with one of its operands; we can
      // save memory by sharing the buffer, rather than allocating a new one.
      // We can only share with the operand buffer if it is about to be freed;
      // we must be the last user of the buffer.
      bool shared = false;
      if (options_.may_reuse_operand_buffers) {
        for (const BufferValue* operand_buffer : operand_buffers_to_free) {
          if (reused_buffers.count(operand_buffer) != 0) {
            continue;
          }
          if (buffer->instruction()->IsUserOf(operand_buffer->instruction()) &&
              buffer->instruction()->opcode() != HloOpcode::kCopy &&
              points_to_analysis.CanShareOperandBufferWithUser(
                  operand_buffer->instruction(), operand_buffer->index(),
                  buffer->instruction(), buffer->index())) {
            VLOG(3) << "  Sharing: " << buffer->ToString() << " with "
                    << operand_buffer->ToString();
            ShareBuffer(buffer, operand_buffer, instruction);
            shared = true;
            reused_buffers.insert(operand_buffer);
            break;
          }
        }
      }

      if (!shared) {
        VLOG(3) << "  Allocating: " << buffer->ToString();
        alloc_size_by_instruction += size_fn_(*buffer);
        Alloc(buffer, instruction);
      }
    }
    // Account for the memory used by subcomputations when estimating the
    // current heap size.
    if (memory_by_computation_ != nullptr) {
      algorithm_->AccountForSubcomputationMemory(
          instruction, alloc_size_by_instruction, *memory_by_computation_);
    }

    // If all computations in the module have been scheduled, we can save memory
    // by running the heap-simulation for sub-computations inline. E.g. the
    // buffers for the condition and body of a kWhile instruction are only live
    // for the duration of the instruction itself.
    //
    // The order that the sub-computations are simulated does not affect
    // correctness; since the whole module has been scheduled, we know that the
    // sub-computations will never be run concurrently.
    if (schedule_ != nullptr) {
      if (instruction->opcode() == HloOpcode::kCall ||
          instruction->opcode() == HloOpcode::kConditional ||
          instruction->opcode() == HloOpcode::kWhile) {
        for (const HloComputation* called_computation :
             instruction->called_computations()) {
          const HloInstructionSequence& called_sequence =
              schedule_->sequence(called_computation);
          TF_RETURN_IF_ERROR(RunComputation(
              *called_computation, called_sequence, points_to_analysis));
        }
      }

      // Other sub-computations (e.g. Map, Reduce, ...) are skipped; they are
      // assigned "thread-local" allocations, meaning their buffers are not
      // allocated up-front at the beginning of the computation.
    }

    // Free buffers that are no longer live.  This is the earliest point that we
    // can de-allocate; right after the last use of the buffer.
    for (const BufferValue* buffer : dead_buffers_to_free) {
      VLOG(3) << "  Freeing dead: " << buffer->ToString();
      Free(buffer, instruction);
    }
    for (const BufferValue* buffer : operand_buffers_to_free) {
      VLOG(3) << "  Freeing operand: " << buffer->ToString();
      Free(buffer, instruction);
    }
  }

  // Any remaining live buffers must be entry parameters or output source
  // buffers, which had a nullptr sentry added.  Free them now, in a
  // deterministic order.
  std::vector<const BufferValue*> to_free;
  to_free.reserve(live_buffers.size());
  for (const auto& buffer_pending : live_buffers) {
    const BufferValue* buffer = buffer_pending.first;
    const flat_hash_set<const HloInstruction*>& pending = buffer_pending.second;
    CHECK_EQ(pending.size(), 1) << *buffer;
    CHECK(*pending.begin() == nullptr) << *buffer;
    to_free.push_back(buffer);
  }

  std::sort(to_free.begin(), to_free.end(),
            [](const BufferValue* x, const BufferValue* y) {
              return x->id() < y->id();
            });
  for (const BufferValue* buffer : to_free) {
    VLOG(3) << "Freeing pending: " << buffer->ToString();
    Free(buffer, root);
  }

  return Status::OK();
}

HeapSimulator::HeapSimulator(
    std::unique_ptr<HeapAlgorithm> algorithm,
    const BufferValue::SizeFunction& size_fn, const Options& options,
    const HloSchedule* schedule,
    const absl::flat_hash_map<const HloComputation*, int64>*
        memory_by_computation)
    : no_fragmentation_stats_(absl::make_unique<NoFragmentationStatsHeap>()),
      algorithm_(std::move(algorithm)),
      size_fn_(size_fn),
      options_(options),
      schedule_(schedule),
      memory_by_computation_(memory_by_computation) {
  debug_trace_.set_whole_module_simulation(schedule_ != nullptr);
}

HeapSimulator::~HeapSimulator() {}

bool HeapSimulator::IgnoreBuffer(const BufferValue* buffer) const {
  // Buffers for constants are ignored unless the alloc_constants option is
  // set. Also ignore buffers that we're not meant to assign.
  //
  // TODO(b/32248867): For consistency, constants should get allocations.
  if (!options_.alloc_constants &&
      buffer->instruction()->opcode() == HloOpcode::kConstant) {
    return true;
  }
  return options_.buffers_to_assign != nullptr &&
         options_.buffers_to_assign->count(buffer) == 0;
}

// Alloc always calls the underlying heap algorithm.
void HeapSimulator::Alloc(const BufferValue* buffer,
                          const HloInstruction* instruction) {
  CHECK(allocated_buffers_.count(buffer) == 0)
      << "Alloc called on allocated buffer: " << *buffer;
  CHECK(freed_buffers_.count(buffer) == 0)
      << "Alloc called on freed buffer: " << *buffer;

  allocated_buffers_.insert(buffer);
  const int64 size = size_fn_(*buffer);
  algorithm_->Alloc(buffer, size);
  no_fragmentation_stats_->Alloc(buffer, size);
  FillDebugTrace(HeapSimulatorTrace::Event::ALLOC, buffer, instruction,
                 nullptr);
}

// Free calls the underlying algorithm for non-shared buffers, and for shared
// buffers whose group liveness has expired.  Shared group liveness is tracked
// by maintaining a refcount; the Free call on the last buffer in the group
// causes Free to be called on the underlying algorithm.
void HeapSimulator::Free(const BufferValue* buffer,
                         const HloInstruction* instruction) {
  auto shared_it = shared_buffers_.find(buffer);
  if (shared_it != shared_buffers_.end()) {
    std::shared_ptr<SharedGroup> group = shared_it->second;
    --group->refcount;
    if (group->refcount > 0) {
      return;
    }
    CHECK_EQ(group->refcount, 0)
        << "Free caused negative refcount on shared buffer: " << *buffer;
    buffer = group->canonical;
  }

  CHECK(allocated_buffers_.count(buffer) > 0)
      << "Free called on non-allocated buffer: " << *buffer;
  CHECK(freed_buffers_.count(buffer) == 0)
      << "Free called on freed buffer: " << *buffer;

  freed_buffers_.insert(buffer);
  const int64 size = size_fn_(*buffer);
  algorithm_->Free(buffer, size);
  no_fragmentation_stats_->Free(buffer, size);

  FillDebugTrace(HeapSimulatorTrace::Event::FREE, buffer, instruction, nullptr);
}

// ShareBuffer associates buffers with their SharedGroup in shared_buffers_.
// The 'buffer' must be a non-allocated, non-freed buffer, just like in calls to
// Alloc.  The 'shared' buffer must be a previously allocated or shared buffer.
// Both 'buffer' and 'shared' will be associated with the same SharedGroup.
void HeapSimulator::ShareBuffer(const BufferValue* buffer,
                                const BufferValue* shared,
                                const HloInstruction* instruction) {
  CHECK_LE(size_fn_(*buffer), size_fn_(*shared))
      << "ShareBuffer oversized buffer" << *buffer << " shared: " << *shared;
  CHECK(allocated_buffers_.count(buffer) == 0)
      << "ShareBuffer called on allocated buffer: " << *buffer;
  CHECK(freed_buffers_.count(buffer) == 0)
      << "ShareBuffer called on freed buffer: " << *buffer;
  CHECK(freed_buffers_.count(shared) == 0)
      << "ShareBuffer called on freed shared buffer: " << *shared;

  const BufferValue* canonical = nullptr;
  auto shared_it = shared_buffers_.find(shared);
  if (shared_it != shared_buffers_.end()) {
    // The 'shared' buffer already has a group; it might be the canonical, but
    // also might not be.  Just add 'buffer' to the existing group.
    std::shared_ptr<SharedGroup> group = shared_it->second;
    canonical = group->canonical;
    ++group->refcount;
    shared_buffers_.emplace(buffer, group);
  } else {
    // The 'shared' buffer doesn't have a group; it must be the canonical.  Add
    // both 'buffer' and 'shared' to a new group.
    CHECK(allocated_buffers_.count(shared) > 0)
        << "ShareBuffer called on non-allocated shared buffer: " << *shared;
    auto group = std::make_shared<SharedGroup>();
    canonical = shared;
    group->canonical = canonical;
    group->refcount = 2;
    shared_buffers_.emplace(buffer, group);
    shared_buffers_.emplace(shared, group);
  }

  FillDebugTrace(HeapSimulatorTrace::Event::SHARE_WITH, buffer, instruction,
                 canonical);
}

HeapSimulator::Result HeapSimulator::Finish() {
  Result result = algorithm_->Finish();

  // Post-process the result to add chunks for shared buffers.  An empty chunk
  // map means that either no buffers were allocated, or the heap was only
  // collecting statistics, e.g. NoFragmentationStatsHeap.
  if (!result.chunk_map.empty()) {
    for (const auto& share_pair : shared_buffers_) {
      const BufferValue* buffer = share_pair.first;
      std::shared_ptr<SharedGroup> group = share_pair.second;
      if (buffer != group->canonical) {
        // The canonical must already exist in the chunk_map, since we called
        // Alloc(canonical) on the underlying algorithm.  Add non-canonical
        // chunks with the same offset as the canonical.
        Chunk chunk = FindOrDie(result.chunk_map, group->canonical);
        chunk.size = size_fn_(*buffer);
        result.chunk_map.emplace(buffer, chunk);
      }
    }
    // If we were told to assign specific buffers, make sure we've assigned
    // exactly that many buffers.
    if (options_.buffers_to_assign != nullptr) {
      CHECK_EQ(options_.buffers_to_assign->size(), result.chunk_map.size());
    }
  }

  // Fragmentation is the difference between the actual and ideal sizes.
  const Result no_frag_result = no_fragmentation_stats_->Finish();
  result.fragmentation_size = result.heap_size - no_frag_result.heap_size;

  // Copy the debug trace we collected to the final result.
  result.debug_trace.Swap(&debug_trace_);

  return result;
}

void HeapSimulator::FillDebugTrace(HeapSimulatorTrace::Event::Kind kind,
                                   const BufferValue* buffer,
                                   const HloInstruction* instruction,
                                   const BufferValue* share_with_canonical) {
  HeapSimulatorTrace::Event* event = debug_trace_.add_events();
  event->set_kind(kind);
  event->set_buffer_id(buffer->id());
  event->set_computation_name(instruction->parent()->name());
  event->set_instruction_name(instruction->name());
  if (kind == HeapSimulatorTrace::Event::SHARE_WITH) {
    CHECK(share_with_canonical != nullptr);
    event->set_share_with_canonical_id(share_with_canonical->id());
  } else {
    CHECK(share_with_canonical == nullptr);
  }
}

void NoFragmentationStatsHeap::Alloc(const BufferValue* buffer, int64 size) {
  current_heap_size_ += size;
  if (current_heap_size_ > max_heap_size_) {
    max_heap_size_ = current_heap_size_;
  }
}

void NoFragmentationStatsHeap::AccountForSubcomputationMemory(
    const HloInstruction* instruction, int64 alloc_size_by_instruction,
    const absl::flat_hash_map<const HloComputation*, int64>&
        memory_by_computation) {
  // We only count the memory usage of the largest subcomputation, instead of
  // adding them all, because subcomputations won't execute in parallel.
  int64 max_subcomputation_bytes = 0;
  for (const auto* c : instruction->called_computations()) {
    auto it = memory_by_computation.find(c);
    if (it != memory_by_computation.end()) {
      int64 subcomputation_bytes = it->second;
      if (subcomputation_bytes > max_subcomputation_bytes) {
        max_subcomputation_bytes = subcomputation_bytes;
      }
    }
  }
  if (max_subcomputation_bytes > 0 &&
      (instruction->opcode() == HloOpcode::kWhile ||
       instruction->opcode() == HloOpcode::kCall ||
       instruction->opcode() == HloOpcode::kConditional)) {
    // The output buffer of while/call/conditional is always aliased with the
    // output buffer of the root instruction in the body. Don't double count.
    max_subcomputation_bytes -= alloc_size_by_instruction;
  }
  max_heap_size_ =
      std::max(max_heap_size_, current_heap_size_ + max_subcomputation_bytes);
}

void NoFragmentationStatsHeap::Free(const BufferValue* buffer, int64 size) {
  current_heap_size_ -= size;
}

HeapSimulator::Result NoFragmentationStatsHeap::Finish() {
  // The result.chunk_map is empty, since we only collect stats, and don't
  // actually compute chunk assignments.
  Result result;
  result.heap_size = max_heap_size_;
  return result;
}

void DecreasingSizeRunsHeap::Alloc(const BufferValue* buffer, int64 size) {
  SetMode(kAlloc);
  run_.emplace_back(Op{buffer, size});
}

void DecreasingSizeRunsHeap::Free(const BufferValue* buffer, int64 size) {
  CHECK(mode_ != kInit) << "Free called on empty heap: " << *buffer;
  SetMode(kFree);
  run_.emplace_back(Op{buffer, size});
}

HeapSimulator::Result DecreasingSizeRunsHeap::Finish() {
  CallAndDrainRun();
  return algorithm_->Finish();
}

void DecreasingSizeRunsHeap::SetMode(Mode mode) {
  if (mode_ != mode) {
    CallAndDrainRun();
    mode_ = mode;
  }
}

void DecreasingSizeRunsHeap::CallAndDrainRun() {
  if (mode_ == kInit) {
    CHECK(run_.empty());
    return;
  }

  // Call ops in the run sorted by decreasing size, breaking ties by buffer id.
  std::sort(run_.begin(), run_.end(), [](const Op& a, const Op& b) {
    if (a.size != b.size) {
      return a.size > b.size;
    }
    return a.buffer->id() < b.buffer->id();
  });
  for (const Op& op : run_) {
    if (mode_ == kAlloc) {
      algorithm_->Alloc(op.buffer, op.size);
    } else {
      algorithm_->Free(op.buffer, op.size);
    }
  }
  run_.clear();
}

void LazyBestFitHeap::Alloc(const BufferValue* buffer, int64 size) {
  // Degenerate case: 0-sized buffers are always allocated at offset 0.
  if (size == 0) {
    result_.chunk_map.emplace(buffer, Chunk{0, 0});
  }

  // First try to allocate from the best-fitting free chunk.
  auto best_fit_it = free_.lower_bound(Chunk{0, size});
  while (best_fit_it != free_.end()) {
    // Account for alignment.
    const Chunk best = *best_fit_it;
    const int64 new_offset = RoundUpToNearest(best.offset, alignment_);
    const int64 new_end = new_offset + size;
    if (new_end > best.chunk_end()) {
      // We don't fit after accounting for alignment.
      ++best_fit_it;
      continue;
    }
    // The buffer is allocated a chunk out of the best-fitting free chunk.
    free_.erase(best_fit_it);
    result_.chunk_map.emplace(buffer, Chunk{new_offset, size});
    // Add remaining portions of the best-fitting free chunk back into free_.
    AddFreeChunk(best.offset, new_offset - best.offset);
    AddFreeChunk(new_end, best.chunk_end() - new_end);
    return;
  }

  // The buffer doesn't completely fit into any existing free chunk.  If the
  // last free chunk is adjacent to the end of the heap, allocate the buffer
  // re-using that space, increasing the heap size.
  //
  // Allocating the buffer now causes the heap to grow by less than the buffer
  // size, whereas if we allocated lazily in Free, the heap would grow by
  // exactly the buffer size.  However it's still a greedy heuristical approach;
  // we might have ended up with a tighter packing by being lazy here.
  //
  // In theory we could also check if we could re-use space from the first free
  // chunk and grow the heap at the front, and choose whether to grow from the
  // front or back based on the amount of re-use.  But that's more complicated,
  // and these are all heuristics anyways, so it isn't implemented.
  for (auto it = free_.begin(); it != free_.end(); ++it) {
    if (it->chunk_end() == result_.heap_size) {
      // Account for alignment in the last free chunk.
      const Chunk last = *it;
      const int64 new_offset = RoundUpToNearest(last.offset, alignment_);
      if (new_offset >= last.chunk_end()) {
        // There's no point in using the last free chunk if alignment causes us
        // to skip over it anyways.
        break;
      }
      // The buffer is allocated a chunk that includes the last free chunk.
      free_.erase(it);
      result_.chunk_map.emplace(buffer, Chunk{new_offset, size});
      // Add remaining portion of the last free chunk back into free_.
      AddFreeChunk(last.offset, new_offset - last.offset);
      // Grow the heap.
      const int64 new_end = new_offset + size;
      CHECK_GT(new_end, result_.heap_size);
      CHECK_LT(new_end, result_.heap_size + size);
      result_.heap_size = new_end;
      return;
    }
  }

  // Otherwise lazily allocate the buffer in Free.
  result_.chunk_map.emplace(buffer, Chunk{kLazyAllocOffset, size});
}

void LazyBestFitHeap::Free(const BufferValue* buffer, int64 size) {
  auto alloc_it = result_.chunk_map.find(buffer);
  CHECK(alloc_it != result_.chunk_map.end())
      << "Free called on non-allocated buffer: " << *buffer;
  Chunk* alloc = &alloc_it->second;
  CHECK_EQ(alloc->size, size) << "Free with mismatched sizes: " << *buffer;
  if (alloc->offset != kLazyAllocOffset) {
    // The buffer was already allocated in Alloc, do a normal free.
    AddFreeChunk(alloc->offset, alloc->size);
  } else {
    // This buffer is lazily allocated, so we *can not* allocate out of existing
    // free chunks, since that might cause interference between buffers.  The
    // buffer is allocated by growing the heap, accounting for alignment.
    alloc->offset = RoundUpToNearest(result_.heap_size, alignment_);
    const int64 new_end = alloc->chunk_end();
    AddFreeChunk(result_.heap_size, new_end - result_.heap_size);
    CHECK_GT(new_end, result_.heap_size);
    CHECK_GE(new_end, result_.heap_size + alloc->size);
    result_.heap_size = new_end;
  }
}

void LazyBestFitHeap::AddFreeChunk(int64 offset, int64 size) {
  if (size <= 0) {
    return;
  }

  // Coalesce the chunk with adjacent free chunks on either side.  We must
  // remove the free chunks from free_, since it's ordered by size.
  Chunk chunk{offset, size};
  for (auto it = free_.begin(); it != free_.end();) {
    if (it->chunk_end() == chunk.offset || it->offset == chunk.chunk_end()) {
      chunk.offset = std::min(chunk.offset, it->offset);
      chunk.size += it->size;
      it = free_.erase(it);
    } else {
      ++it;
    }
  }

  // This is the only place we add free chunks to free_.  It maintains the
  // invariant that all free chunks are disjoint and non-adjacent.
  free_.emplace(chunk);
}

HeapSimulator::Result LazyBestFitHeap::Finish() {
  if (!free_.empty()) {
    // When Finish is called, all calls to Alloc must have had corresponding
    // calls to Free, which will result in a single free chunk [0, heap_size).
    CHECK_EQ(free_.size(), 1);
    CHECK_EQ(free_.begin()->offset, 0);
    CHECK_EQ(free_.begin()->size, result_.heap_size);
  }
  return result_;
}

void GlobalDecreasingSizeBestFitHeap::Alloc(const BufferValue* buffer,
                                            int64 size) {
  // Degenerate case: 0-sized buffers are always allocated at offset 0.
  if (size == 0) {
    result_.chunk_map.emplace(buffer, Chunk{0, 0});
    return;
  }
  auto emplace_result = buffer_intervals_.emplace(
      buffer, BufferInterval{buffer, size, current_time_, -1});
  DCHECK(emplace_result.second);
  ++current_time_;
}

void GlobalDecreasingSizeBestFitHeap::Free(const BufferValue* buffer,
                                           int64 size) {
  // Degenerate case: 0-sized buffers are always allocated at offset 0.
  if (size == 0) {
    return;
  }
  BufferInterval& buffer_interval = FindOrDie(buffer_intervals_, buffer);
  DCHECK_EQ(buffer_interval.buffer, buffer);
  DCHECK_EQ(buffer_interval.size, size);
  DCHECK_EQ(buffer_interval.end, -1);
  buffer_interval.end = current_time_;
  ++current_time_;
}

namespace {

// Node in BufferIntervalTree that stores the alloc and free times of a buffer,
// and the chunk assigned to it.
struct BufferIntervalTreeNode {
  // Alloc time.
  int64 start;
  // Free time.
  int64 end;
  // Maximum free time of all nodes in the subtree where this node is the root.
  int64 subtree_end;
  // Allocated chunk for the buffer.
  HeapSimulator::Chunk chunk;
  // Left child.
  BufferIntervalTreeNode* left;
  // Right child.
  BufferIntervalTreeNode* right;
};

// An interval tree that can query buffers overlapping in time.
class BufferIntervalTree {
 public:
  explicit BufferIntervalTree(int capacity) : node_storage_(capacity) {}

  using Chunk = HeapSimulator::Chunk;

  // Adds a buffer to the interval tree, with the time interval and allocated
  // chunk specified.
  void Add(int64 start, int64 end, const Chunk& chunk) {
    int index = node_count_;
    DCHECK_LT(index, node_storage_.size());
    ++node_count_;

    node_storage_[index] =
        BufferIntervalTreeNode{start, end, end, chunk, nullptr, nullptr};

    if (index == 0) {
      // This is root.
      return;
    }

    BufferIntervalTreeNode* parent = &node_storage_[0];
    while (true) {
      parent->subtree_end = std::max(parent->subtree_end, end);
      if (parent->start > start) {
        if (parent->left == nullptr) {
          parent->left = &node_storage_[index];
          return;
        }
        parent = parent->left;
      } else {
        if (parent->right == nullptr) {
          parent->right = &node_storage_[index];
          return;
        }
        parent = parent->right;
      }
    }
  }

  // Returns vector of allocated chunks that overlap with the given time
  // interval.
  std::vector<Chunk> ChunksOverlappingInTime(int64 start, int64 end) {
    std::vector<Chunk> result;
    if (node_count_ == 0) {
      return result;
    }
    std::vector<BufferIntervalTreeNode*> visiting_stack;
    visiting_stack.push_back(&node_storage_[0]);
    while (!visiting_stack.empty()) {
      BufferIntervalTreeNode* top = visiting_stack.back();
      visiting_stack.pop_back();
      if (start > top->subtree_end) {
        continue;
      }
      if (top->left != nullptr) {
        visiting_stack.push_back(top->left);
      }
      if (top->start <= end && top->end >= start) {
        result.push_back(top->chunk);
      }
      if (end < top->start) {
        continue;
      }
      if (top->right != nullptr) {
        visiting_stack.push_back(top->right);
      }
    }
    return result;
  }

 private:
  int64 node_count_ = 0;
  std::vector<BufferIntervalTreeNode> node_storage_;
};

}  // namespace

HeapSimulator::Result GlobalDecreasingSizeBestFitHeap::Finish() {
  std::vector<BufferInterval> sorted_buffer_intervals;
  for (auto& entry : buffer_intervals_) {
    sorted_buffer_intervals.push_back(entry.second);
  }
  std::sort(sorted_buffer_intervals.begin(), sorted_buffer_intervals.end(),
            [](const BufferInterval& x, const BufferInterval& y) {
              if (x.size != y.size) {
                return x.size > y.size;
              }
              if (x.end - x.start != y.end - y.start) {
                return x.end - x.start > y.end - y.start;
              }
              return x.buffer->id() < y.buffer->id();
            });

  BufferIntervalTree interval_tree(sorted_buffer_intervals.size());
  for (auto& buffer_interval : sorted_buffer_intervals) {
    auto chunks_overlapping_in_time = interval_tree.ChunksOverlappingInTime(
        buffer_interval.start, buffer_interval.end);
    std::sort(
        chunks_overlapping_in_time.begin(), chunks_overlapping_in_time.end(),
        [](const Chunk& x, const Chunk& y) { return x.offset < y.offset; });

    // Find the minimum free chunk that can hold this buffer.
    Chunk min_fit_chunk{-1, INT64_MAX};
    auto use_free_chunk_if_smaller = [&](int64 free_offset, int64 free_size) {
      if (free_size < buffer_interval.size) {
        return;
      }

      if (free_size < min_fit_chunk.size) {
        min_fit_chunk = {free_offset, free_size};
      }
    };

    int64 offset = 0;
    for (auto& chunk : chunks_overlapping_in_time) {
      if (offset < chunk.offset) {
        use_free_chunk_if_smaller(offset, chunk.offset - offset);
      }
      offset =
          std::max(offset, RoundUpToNearest(chunk.chunk_end(), alignment_));
    }
    use_free_chunk_if_smaller(offset, result_.heap_size - offset);

    if (min_fit_chunk.offset == -1) {
      // Increase the heap size to fit in the last free chunk.
      result_.heap_size = offset + buffer_interval.size;
      min_fit_chunk = {offset, buffer_interval.size};
    }

    min_fit_chunk.size = buffer_interval.size;
    const auto emplace_result =
        result_.chunk_map.emplace(buffer_interval.buffer, min_fit_chunk);
    DCHECK(emplace_result.second);

    interval_tree.Add(buffer_interval.start, buffer_interval.end,
                      min_fit_chunk);
  }
  return result_;
}

HeapSimulator::Result ChooseBestHeapAlgorithm::Finish() {
  DCHECK(!algorithms_.empty());
  std::vector<Result> results(algorithms_.size());
  int64 min_size = INT64_MAX;
  int min_size_index = -1;
  for (int i = 0; i < algorithms_.size(); ++i) {
    results[i] = algorithms_[i]->Finish();
    if (results[i].heap_size < min_size) {
      min_size = results[i].heap_size;
      min_size_index = i;
    }
  }

  DCHECK_GE(min_size_index, 0);
  return results[min_size_index];
}

}  // namespace xla
