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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_HLO_REACHABILITY_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_HLO_REACHABILITY_H_

#include <cstdio>
#include <list>
#include <vector>

#include "absl/base/casts.h"
#include "absl/container/flat_hash_map.h"
#include "absl/types/span.h"
#include "tensorflow/compiler/xla/map_util.h"
#include "tensorflow/compiler/xla/service/hlo_computation.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_module.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/platform/types.h"

namespace xla {

// A class for representing reachability between HloInstructions.
//
// It has an adjacency matrix and it is up to the user of the class to set the
// adjacency matrix such that it represents reachability, i.e. such that it is
// transitive. That the graph be transitive is thus not an invariant of this
// class, but it is required for the name of the class and its methods to make
// sense.
class HloReachabilityMap {
 public:
  // Sets up a graph with no edges and where the nodes correspond to the given
  // instructions.
  explicit HloReachabilityMap(
      absl::Span<const HloInstruction* const> instructions);

  // Computes and returns the reachability between HLO instructions in the
  // computation. The returned HloReachabilityMap is constructed such that
  // HloReachabilityMap::IsReachable(a, b) returns true iff there exists a
  // directed path (from producer to consumer) from 'a' to 'b'. Both data
  // dependencies (operands) and control dependencies are considered for
  // reachability. Trivially an instruction is reachable from itself.
  static std::unique_ptr<HloReachabilityMap> Build(
      const HloComputation* computation);

  // Set the reachability set of 'instruction' to the union of the reachability
  // sets of 'inputs'. Upon return, IsReachable(x, instruction) where
  // 'x' is not 'instruction' will return true iff IsReachable(x, input) is true
  // for some 'input' in 'inputs'. Also sets 'instruction' to be reachable from
  // itself. Returns whether the reachability set of 'instruction' changed.
  //
  // !!! THIS FUNCTION DOES NOT COMPUTE REACHABILITY !!! It sets the adjacency
  // vector in the internal graph of this HloReachabilityMap for the given
  // instruction and does not transitively update any other part of the
  // adjacency matrix.
  bool SetReachabilityToUnion(absl::Span<const HloInstruction* const> inputs,
                              const HloInstruction* instruction);

  // As above, but faster because it does not check if the reachability changed.
  void FastSetReachabilityToUnion(
      absl::Span<const HloInstruction* const> inputs,
      const HloInstruction* instruction);

  // Sets entry so that IsReachable(a, b) will return true
  //
  // !!! THIS FUNCTION DOES NOT COMPUTE REACHABILITY !!! It sets the adjacency
  // matrix in the internal graph of this HloReachabilityMap to have an edge
  // from a to b and does not transitively update any other part of the
  // adjacency matrix.
  void SetReachable(const HloInstruction* a, const HloInstruction* b);

  // Updates the given reachability map after the immediate predecessor set
  // (operands and control predecessors) of 'instruction' has changed.
  void UpdateReachabilityThroughInstruction(const HloInstruction* instruction);

  // Returns true if "b" is reachable from "a"
  //
  // Note that this function only correctly answers queries about reachability
  // if the set of edges that have been provided to this class are transitive.
  bool IsReachable(const HloInstruction* a, const HloInstruction* b) const;

  // Returns true if "b" is reachable from "a" or "a" is reachable from "b"
  //
  // Note that this function only correctly answers queries about reachability
  // if the set of edges that have been provided to this class are transitive.
  bool IsConnected(const HloInstruction* a, const HloInstruction* b) const;

  // Checks if an instruction is in the Reachability map.
  bool IsPresent(const HloInstruction* a) const {
    return indices_.contains(GetKey(a));
  }

 private:
  // A bit-vector implementation specialized for this use case which provides a
  // fast bitwise OR operation not available in tensorflow::gtl::BitMap.
  class BitVector {
   public:
    BitVector() = default;
    BitVector(size_t size)
        : size_(size), vector_((size + kBits - 1) / kBits, 0) {}

    // Return the bit at the given index.
    bool Get(size_t index) const {
      DCHECK(index >= 0 && index < size_);
      return vector_[index / kBits] & (1ull << (index % kBits));
    }

    // Set the bit at the given index.
    void Set(size_t index) {
      DCHECK(index >= 0 && index < size_);
      vector_[index / kBits] |= 1ull << (index % kBits);
    }

    // Set this bitvector to the Logical OR of this bitvector and 'other'.
    void OrWith(const BitVector& other) {
      for (size_t i = 0; i < vector_.size(); ++i) {
        vector_[i] |= other.vector_[i];
      }
    }

    // Set the bitvector to all zeros.
    void SetToZero() { std::fill(vector_.begin(), vector_.end(), 0); }

    bool operator==(const BitVector& other) const {
      return vector_ == other.vector_;
    }
    bool operator!=(const BitVector& other) const {
      return vector_ != other.vector_;
    }

   private:
    using Word = uint64;
    static const size_t kBits = 64;

    // Number of bits in the bitvector.
    size_t size_;

    std::vector<Word> vector_;
  };

  // Return the bitvector storing the reachability-to of the given instruction.
  const BitVector& GetBitVector(const HloInstruction* instruction) const {
    return bit_vectors_[GetIndex(instruction)];
  }
  BitVector& GetBitVector(const HloInstruction* instruction) {
    return bit_vectors_[GetIndex(instruction)];
  }

  // Helper for SetReachabilityToUnion/FastSetReachabilityToUnion.
  void SetReachabilityToUnionHelper(
      absl::Span<const HloInstruction* const> inputs,
      const HloInstruction* instruction, BitVector* bit_vector);

  uint64 GetKey(const HloInstruction* instruction) const {
    uint64 unique_id = absl::bit_cast<uint32>(instruction->unique_id());
    uint64 module_id =
        absl::bit_cast<uint32>(instruction->parent()->parent()->unique_id());
    return (module_id << 32) | unique_id;
  }
  // Return the index of the given instruction. The value is used to index into
  // the vector of BitVectors and the BitVectors themselves.
  int GetIndex(const HloInstruction* instruction) const {
    return FindOrDie(indices_, GetKey(instruction));
  }

  // The number of instructions in the reachability map.
  const size_t size_;

  // Dense assignment from HloInstruction::unique_id to number. These numbers
  // index into the bit_vectors_ vector and into the bits within a BitVector.
  absl::flat_hash_map<uint64, int> indices_;

  // Bitvectors holding the reachability to each instruction. The bit vector for
  // instruction X includes ones for each instruction which X is reachable from.
  std::vector<BitVector> bit_vectors_;

  // A temporary used by SetReachabilityToUnion to avoid an allocation with each
  // call to the method.
  BitVector tmp_bit_vector_;
};

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_HLO_REACHABILITY_H_
