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

#include "tensorflow/compiler/xla/service/hlo_liveness_analysis.h"

#include <deque>

#include "absl/memory/memory.h"
#include "absl/strings/str_cat.h"
#include "tensorflow/compiler/xla/map_util.h"
#include "tensorflow/compiler/xla/service/call_graph.h"
#include "tensorflow/compiler/xla/service/hlo_computation.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_module.h"
#include "tensorflow/compiler/xla/service/hlo_opcode.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/status.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/platform/logging.h"

namespace xla {
namespace {

using Worklist = std::deque<const HloInstruction*>;
using Workset = std::unordered_set<const HloInstruction*>;

void AddToWorklist(const HloInstruction* instruction, Worklist* worklist,
                   Workset* workset) {
  if (workset->count(instruction) == 0) {
    worklist->push_back(instruction);
    workset->insert(instruction);
    VLOG(3) << "ADD instruction: " << instruction->name();
  }
}

using VisitorFunction = std::function<void(const ShapeIndex& /*index*/)>;

void ForEachLiveIndex(const ShapeTree<bool>& index_tree,
                      const VisitorFunction& func) {
  index_tree.ForEachElement([&](const ShapeIndex& shape_index, bool live) {
    if (live) {
      func(shape_index);
    }
  });
}

// Marks 'instruction' output live at 'shape_index'.
// Adds to 'worklist' iff:
// *) 'instruction' is not already on worklist.
// *) 'shape_index' has not yet been visited.
void MarkLiveAtIndex(const HloInstruction* instruction,
                     const ShapeIndex& shape_index,
                     HloLivenessAnalysis::HloIndexMap* live_index_map,
                     Worklist* worklist, Workset* workset) {
  auto it = live_index_map->find(instruction);
  if (it == live_index_map->end()) {
    auto it_added = live_index_map->emplace(
        std::piecewise_construct, std::forward_as_tuple(instruction),
        std::forward_as_tuple(instruction->shape(), /*init_value=*/false));
    it = it_added.first;
  }
  if (it->second.element(shape_index) == false) {
    AddToWorklist(instruction, worklist, workset);
    *it->second.mutable_element(shape_index) = true;
    VLOG(3) << "MARK instruction: " << instruction->name()
            << " shape_index: " << shape_index.ToString();
  }
}

// Marks 'instruction' live at all shape indices in its output.
void MarkLiveAtAllIndices(const HloInstruction* instruction,
                          HloLivenessAnalysis::HloIndexMap* live_index_map,
                          Worklist* worklist, Workset* workset) {
  bool add_to_worklist = false;
  auto it = live_index_map->find(instruction);
  if (it == live_index_map->end()) {
    live_index_map->emplace(
        std::piecewise_construct, std::forward_as_tuple(instruction),
        std::forward_as_tuple(instruction->shape(), /*init_value=*/true));
    add_to_worklist = true;
  } else {
    ShapeUtil::ForEachSubshape(
        instruction->shape(),
        [&](const Shape& sub_shape, const ShapeIndex& shape_index) {
          if (it->second.element(shape_index) == false) {
            add_to_worklist = true;
            *it->second.mutable_element(shape_index) = true;
            VLOG(3) << "MARK instruction: " << instruction->name()
                    << " shape_index: " << shape_index.ToString();
          }
        });
  }
  if (add_to_worklist) {
    AddToWorklist(instruction, worklist, workset);
  }
}

// Propagates liveness through Tuple instructions.
// *) For each tuple operand:
//   *) For tuple output shape index associated with operand:
//     *) Propgate live shape indices to tuple operand at the associated
//        shape index in the operands output, and add to worklist.
void PropagateLivenessThroughTuple(
    const HloInstruction* instruction,
    HloLivenessAnalysis::HloIndexMap* live_index_map, Worklist* worklist,
    Workset* workset) {
  CHECK_EQ(instruction->opcode(), HloOpcode::kTuple);
  for (int64 operand_index = 0; operand_index < instruction->operand_count();
       ++operand_index) {
    const ShapeTree<bool>& index_tree = FindOrDie(*live_index_map, instruction);
    ForEachLiveIndex(index_tree, [&](const ShapeIndex& shape_index) {
      if (shape_index.empty() || shape_index[0] != operand_index) {
        return;
      }
      // Mark top-level index of operand at 'operand_index'.
      MarkLiveAtIndex(instruction->operand(operand_index), {}, live_index_map,
                      worklist, workset);
      // Mark sub-shape index of operand at 'operand_index'.
      ShapeIndex operand_shape_index;
      for (int i = 1; i < shape_index.size(); ++i) {
        operand_shape_index.push_back(shape_index[i]);
      }
      MarkLiveAtIndex(instruction->operand(operand_index), operand_shape_index,
                      live_index_map, worklist, workset);
    });
  }
}

// Propagates liveness through GetTupleElement instructions.
// *) For each live index in GetTupleElement output, mark output of GTE operand
//    at associated shape index in its output, and add to worklist.
void PropagateLivenessThroughGTE(
    const HloInstruction* instruction,
    HloLivenessAnalysis::HloIndexMap* live_index_map, Worklist* worklist,
    Workset* workset) {
  CHECK_EQ(instruction->opcode(), HloOpcode::kGetTupleElement);
  // Mark operand top-level index.
  MarkLiveAtIndex(instruction->operand(0), {}, live_index_map, worklist,
                  workset);
  const ShapeTree<bool>& index_tree = FindOrDie(*live_index_map, instruction);
  // Propagate live shape indices along GTE -> Tuple edge.
  ForEachLiveIndex(index_tree, [&](const ShapeIndex& shape_index) {
    ShapeIndex operand_shape_index(shape_index);
    operand_shape_index.push_front(instruction->tuple_index());
    MarkLiveAtIndex(instruction->operand(0), operand_shape_index,
                    live_index_map, worklist, workset);
  });
}

// Propagates liveness through While instructions.
// *) For each live index in While output, mark shape index of while.body.root
//    and while.operand (adding each to worklist).
// *) Mark while.cond.root and add to worklist.
void PropagateLivenessThroughWhile(
    const HloInstruction* instruction,
    HloLivenessAnalysis::HloIndexMap* live_index_map, Worklist* worklist,
    Workset* workset) {
  CHECK_EQ(instruction->opcode(), HloOpcode::kWhile);
  const ShapeTree<bool>& index_tree = FindOrDie(*live_index_map, instruction);

  ForEachLiveIndex(index_tree, [&](const ShapeIndex& shape_index) {
    // Propagate liveness to while body computation root instruction.
    MarkLiveAtIndex(instruction->while_body()->root_instruction(), shape_index,
                    live_index_map, worklist, workset);
    // Propagate liveness to tuple-shaped operand.
    MarkLiveAtIndex(instruction->operand(0), shape_index, live_index_map,
                    worklist, workset);
  });

  // Propagate liveness to while condition computation root instruction.
  MarkLiveAtIndex(instruction->while_condition()->root_instruction(), {},
                  live_index_map, worklist, workset);
}

// Propagates liveness out of Parameter instructions to callers and aliasing
// positions. This can occur if liveness propagates to a parameter in the
// while.condition computation, requiring liveness to propagate out to caller
// callsite while (and while.body.root).
void PropagateLivenessToParameterCallers(
    const HloInstruction* instruction,
    HloLivenessAnalysis::HloIndexMap* live_index_map, Worklist* worklist,
    Workset* workset, CallGraph* call_graph) {
  CHECK_EQ(instruction->opcode(), HloOpcode::kParameter);
  const CallGraphNode& call_graph_node =
      call_graph->GetNode(instruction->parent());
  if (call_graph_node.context() == CallContext::kSequential) {
    for (const CallSite& callsite : call_graph_node.caller_callsites()) {
      if (callsite.instruction()->opcode() == HloOpcode::kWhile) {
        auto* xla_while = callsite.instruction();
        const ShapeTree<bool>& index_tree =
            FindOrDie(*live_index_map, instruction);
        ForEachLiveIndex(index_tree, [&](const ShapeIndex& shape_index) {
          // Propagate liveness to while result{shape_index}
          MarkLiveAtIndex(xla_while, shape_index, live_index_map, worklist,
                          workset);
          // Propagate liveness to while body root{shape_index}.
          MarkLiveAtIndex(xla_while->while_body()->root_instruction(),
                          shape_index, live_index_map, worklist, workset);
          // Propagate liveness to operand(0){shape_index}.
          MarkLiveAtIndex(xla_while->operand(0), shape_index, live_index_map,
                          worklist, workset);
        });
      }
    }
  }
}

// Makes sure that if a live instruction is within a computation used in control
// flow operations, we mark live even other related instructions.
void PropagateLivenessThroughControlFlow(
    const HloInstruction* instruction,
    HloLivenessAnalysis::HloIndexMap* live_index_map, Worklist* worklist,
    Workset* workset, CallGraph* call_graph) {
  const CallGraphNode& call_graph_node =
      call_graph->GetNode(instruction->parent());
  if (call_graph_node.context() == CallContext::kSequential) {
    for (const CallSite& callsite : call_graph_node.caller_callsites()) {
      HloInstruction* caller = callsite.instruction();
      if (caller->opcode() == HloOpcode::kWhile) {
        // If a live instruction is within the %while body or condition
        // computation, mark the predicate value returned by the condition
        // computation live as well.
        MarkLiveAtIndex(caller->while_condition()->root_instruction(), {},
                        live_index_map, worklist, workset);
      } else if (caller->opcode() == HloOpcode::kConditional) {
        // If a live instruction is within the true or false branches of a
        // conditional, we mark the predicate operand live as well.
        MarkLiveAtIndex(caller->operand(0), {}, live_index_map, worklist,
                        workset);
      }
    }
  }
}

}  // namespace

HloLivenessAnalysis::HloLivenessAnalysis(const HloModule& module)
    : module_(module), call_graph_(CallGraph::Build(&module)) {}

// Runs liveness analysis on 'module_'.
// Initializes worklist with entry root instruction (and any instruction with
// side-effects), marking all of their output shape indices live.
// Visits elements on worklist, propagating liveness from an instructions
// live output shape indices to its called computations and operands.
void HloLivenessAnalysis::RunAnalysis() {
  Worklist worklist;
  Workset workset;
  // Add entry compuation root instruction.
  MarkLiveAtAllIndices(module_.entry_computation()->root_instruction(),
                       &live_index_map_, &worklist, &workset);
  for (auto* computation : module_.computations()) {
    for (auto* instruction : computation->instructions()) {
      if (instruction->HasSideEffectNoRecurse()) {
        // Add instructions with side effects.
        MarkLiveAtAllIndices(instruction, &live_index_map_, &worklist,
                             &workset);
      }
    }
  }

  while (!worklist.empty()) {
    const HloInstruction* instruction = worklist.front();
    worklist.pop_front();
    workset.erase(workset.find(instruction));
    VLOG(1) << "VISIT instruction: " << instruction->name();

    if (instruction->opcode() == HloOpcode::kTuple) {
      PropagateLivenessThroughTuple(instruction, &live_index_map_, &worklist,
                                    &workset);
    } else if (instruction->opcode() == HloOpcode::kGetTupleElement) {
      PropagateLivenessThroughGTE(instruction, &live_index_map_, &worklist,
                                  &workset);
    } else if (instruction->opcode() == HloOpcode::kWhile) {
      PropagateLivenessThroughWhile(instruction, &live_index_map_, &worklist,
                                    &workset);
    } else if (instruction->opcode() == HloOpcode::kParameter) {
      PropagateLivenessToParameterCallers(instruction, &live_index_map_,
                                          &worklist, &workset,
                                          call_graph_.get());
    } else {
      // Propagate liveness to called computations.
      for (auto* called_computation : instruction->called_computations()) {
        MarkLiveAtAllIndices(called_computation->root_instruction(),
                             &live_index_map_, &worklist, &workset);
      }
      // Propagate liveness to operands.
      for (HloInstruction* operand : instruction->operands()) {
        MarkLiveAtAllIndices(operand, &live_index_map_, &worklist, &workset);
      }
    }
    PropagateLivenessThroughControlFlow(instruction, &live_index_map_,
                                        &worklist, &workset, call_graph_.get());
  }
}

bool HloLivenessAnalysis::IsLive(const HloInstruction* instruction,
                                 const ShapeIndex& shape_index) const {
  if (ContainsKey(live_index_map_, instruction)) {
    return FindOrDie(live_index_map_, instruction).element(shape_index);
  }
  return false;
}

/* static */
StatusOr<std::unique_ptr<HloLivenessAnalysis>> HloLivenessAnalysis::Run(
    const HloModule& module) {
  VLOG(1) << "HloLivenessAnalysis::Run on module " << module.name();
  XLA_VLOG_LINES(2, module.ToString());

  auto liveness_analysis = absl::WrapUnique(new HloLivenessAnalysis(module));

  liveness_analysis->RunAnalysis();

  return std::move(liveness_analysis);
}

}  // namespace xla
