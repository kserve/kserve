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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_HLO_DOMAIN_MAP_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_HLO_DOMAIN_MAP_H_

#include <memory>
#include <vector>

#include "absl/container/flat_hash_map.h"
#include "absl/container/flat_hash_set.h"
#include "tensorflow/compiler/xla/service/hlo_computation.h"
#include "tensorflow/compiler/xla/service/hlo_domain_metadata.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_module.h"
#include "tensorflow/compiler/xla/statusor.h"
#include "tensorflow/core/lib/core/status.h"

namespace xla {

// The HloDomainMap splits a set of instructions within a module or computation,
// into different domains, separated by kDomain instructions.
// A domain is composed by a set of instructions which can reach each other via
// operand/user edges, without crossing a kDomain insutrction of a given kind.
// A domain never crosses computation boundaries.
class HloDomainMap {
 public:
  // Creates a new HloDomainMap, creating all the domains within the input
  // computation, of the given kind. If domain_kind is not empty, only the
  // kDomain instructions of domain_kind will be considered as separators.
  // Otherwise every kDomain instruction will be splitting domains.
  static StatusOr<std::unique_ptr<HloDomainMap>> Create(
      HloComputation* computation, string domain_kind);

  // Creates a new HloDomainMap, creating all the domains within the input
  // module, of the given kind. If domain_kind is not empty, only the
  // kDomain instructions of domain_kind will be considered as separators.
  // Otherwise every kDomain instruction will be splitting domains.
  static StatusOr<std::unique_ptr<HloDomainMap>> Create(HloModule* module,
                                                        string domain_kind);

  // Retrieves all the domains the input module or computation are composed by.
  const std::vector<std::unique_ptr<DomainMetadata::Domain>>& GetDomains()
      const {
    return instruction_domains_;
  }

  // Checks whether two instructions are within the same domain.
  bool InSameDomain(const HloInstruction* instruction1,
                    const HloInstruction* instruction2) const;

  // Checks whether instruction is a kDomain instruction of the kind we are
  // currently processing.
  bool IsDomainInstruction(const HloInstruction* instruction) const;

  // Retrieves the domain identifier of the instruction, or -1 in case
  // instruction is not found within any domain.
  int64 GetDomainId(const HloInstruction* instruction) const;

  // Returns the unique id of the domain metadata for the domain the given
  // instruction belongs to. The given instruction must not be a kDomain
  // instruction since each domain instruction is associated with 2 domains.
  int64 GetDomainMetadataId(const HloInstruction* instruction) const;

 private:
  // Map used for representing instruction ordering, i.e.
  // order_map[a] < order_map[b] means a must be ordered before b.
  using InstructionOrderMap = absl::flat_hash_map<const HloInstruction*, int64>;

  HloDomainMap(string domain_kind) : domain_kind_(std::move(domain_kind)) {}

  // Check if the kDomain instruction is facing (via its operand link) another
  // kDomain instruction of the same kind, hence defining an empty domain.
  // If that is the case, create the empty domain and call the proper
  // normalizer.
  Status TryProcessEmptyDomain(HloInstruction* instruction);

  Status Populate(HloComputation* computation);

  // Inserts the provided domain into the ones tracked by this object,
  // creating a new domain ID.
  Status InsertDomain(std::unique_ptr<DomainMetadata::Domain> domain);

  // From the given instruction, epxands operand and user wise, the set of
  // instructions which can be reached without crossing a kDomain instruction
  // of the kind specified by domain_kind_.
  // The domain data structure will be populated with all the reached
  // instructions, and the boundaries of the domain, with the kDomain
  // instructions encountered while expanding the reach.
  Status ExpandDomain(HloInstruction* instruction,
                      DomainMetadata::Domain* domain) const;

  // Creates a domain data structure using the ExpandDomain() API.
  StatusOr<std::unique_ptr<DomainMetadata::Domain>> CreateDomain(
      HloInstruction* instruction,
      const InstructionOrderMap& instructions_order) const;

  // Out of an instruction set, returns a vector of all the ones which are not
  // a kDomain kind.
  static std::vector<HloInstruction*> MakeNonDomainInstructions(
      const absl::flat_hash_set<HloInstruction*>& instruction_set,
      const InstructionOrderMap& instructions_order);

  // Populates domain_metadata_id_ that maps each HloInstruction to the unique
  // ID of its associated domain metatadata.
  Status PopulateDomainMetadataMap();

  string domain_kind_;
  std::vector<std::unique_ptr<DomainMetadata::Domain>> instruction_domains_;
  absl::flat_hash_map<const HloInstruction*, int64> instruction_to_domain_;
  absl::flat_hash_map<const HloInstruction*, int64> domain_metadata_id_;
};

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_HLO_DOMAIN_MAP_H_
