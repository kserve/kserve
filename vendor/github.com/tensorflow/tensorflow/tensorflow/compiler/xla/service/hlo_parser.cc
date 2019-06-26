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

#include "tensorflow/compiler/xla/service/hlo_parser.h"

#include "absl/algorithm/container.h"
#include "absl/memory/memory.h"
#include "absl/strings/str_cat.h"
#include "absl/strings/str_format.h"
#include "absl/strings/str_join.h"
#include "absl/strings/str_split.h"
#include "tensorflow/compiler/xla/literal.h"
#include "tensorflow/compiler/xla/literal_util.h"
#include "tensorflow/compiler/xla/service/hlo_domain_metadata.h"
#include "tensorflow/compiler/xla/service/hlo_instructions.h"
#include "tensorflow/compiler/xla/service/hlo_opcode.h"
#include "tensorflow/compiler/xla/service/hlo_schedule.h"
#include "tensorflow/compiler/xla/service/hlo_sharding_metadata.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/core/lib/gtl/map_util.h"

namespace xla {

namespace {

using absl::nullopt;
using absl::optional;
using absl::StrAppend;
using absl::StrCat;
using absl::StrFormat;
using absl::StrJoin;

const double kF16max = 65504;

// Creates and returns a schedule created using the order of the instructions in
// the HloComputation::instructions() vectors in the module.
HloSchedule ScheduleFromInstructionOrder(HloModule* module) {
  HloSchedule schedule(module);
  for (HloComputation* computation : module->computations()) {
    if (!computation->IsFusionComputation()) {
      for (HloInstruction* instruction : computation->instructions()) {
        schedule.GetOrCreateSequence(computation).push_back(instruction);
      }
    }
  }
  return schedule;
}

// Parser for the HloModule::ToString() format text.
class HloParser {
 public:
  using LocTy = HloLexer::LocTy;

  explicit HloParser(absl::string_view str) : lexer_(str) {}

  // Runs the parser and constructs the resulting HLO in the given (empty)
  // HloModule. Returns false if an error occurred.
  Status Run(HloModule* module);

  // Returns the error information.
  string GetError() const { return StrJoin(error_, "\n"); }

  // Stand alone parsing utils for various aggregate data types.
  StatusOr<HloSharding> ParseShardingOnly();
  StatusOr<Window> ParseWindowOnly();
  StatusOr<ConvolutionDimensionNumbers> ParseConvolutionDimensionNumbersOnly();
  StatusOr<PaddingConfig> ParsePaddingConfigOnly();

 private:
  using InstrNameTable =
      std::unordered_map<string, std::pair<HloInstruction*, LocTy>>;

  // Returns the map from the instruction name to the instruction itself and its
  // location in the current scope.
  InstrNameTable& current_name_table() { return scoped_name_tables_.back(); }

  // Locates an instruction with the given name in the current_name_table() or
  // returns nullptr.
  //
  // When the name is not found or name is empty, if create_missing_instruction_
  // hook is registered and a "shape" is provided, the hook will be called to
  // create an instruction. This is useful when we reify parameters as they're
  // resolved; i.e. for ParseSingleInstruction.
  std::pair<HloInstruction*, LocTy>* FindInstruction(
      const string& name, const optional<Shape>& shape = nullopt);

  // Parse a single instruction worth of text.
  bool ParseSingleInstruction(HloModule* module);

  // ParseXXX returns false if an error occurred.
  bool ParseHloModule(HloModule* module);

  bool ParseComputations(HloModule* module);
  bool ParseComputation(HloComputation** entry_computation);
  bool ParseInstructionList(HloComputation** computation,
                            const string& computation_name);
  bool ParseInstruction(HloComputation::Builder* builder, string* root_name);
  bool ParseInstructionRhs(HloComputation::Builder* builder, const string& name,
                           LocTy name_loc);
  bool ParseControlPredecessors(HloInstruction* instruction);
  bool ParseLiteral(Literal* literal, const Shape& shape);
  bool ParseTupleLiteral(Literal* literal, const Shape& shape);
  bool ParseNonTupleLiteral(Literal* literal, const Shape& shape);
  bool ParseDenseLiteral(Literal* literal, const Shape& shape);
  bool ParseSparseLiteral(Literal* literal, const Shape& shape);
  template <typename LiteralNativeT>
  bool ParseSparseLiteralHelper(Literal* literal, const Shape& shape);

  // Sets the sub-value of literal at the given index to the given value. The
  // literal's shape must have the default layout.
  bool SetValueInLiteral(tensorflow::int64 value,
                         tensorflow::int64 linear_index, Literal* literal);
  bool SetValueInLiteral(double value, tensorflow::int64 linear_index,
                         Literal* literal);
  bool SetValueInLiteral(bool value, tensorflow::int64 linear_index,
                         Literal* literal);
  template <typename LiteralNativeT, typename ParsedElemT>
  bool SetValueInLiteralHelper(ParsedElemT value,
                               tensorflow::int64 linear_index,
                               Literal* literal);

  bool ParseOperands(std::vector<HloInstruction*>* operands);
  // Fills parsed operands into 'operands' and expects a certain number of
  // operands.
  bool ParseOperands(std::vector<HloInstruction*>* operands,
                     const int expected_size);

  // Describes the start, limit, and stride on every dimension of the operand
  // being sliced.
  struct SliceRanges {
    std::vector<tensorflow::int64> starts;
    std::vector<tensorflow::int64> limits;
    std::vector<tensorflow::int64> strides;
  };

  // The data parsed for the kDomain instruction.
  struct DomainData {
    std::unique_ptr<DomainMetadata> entry_metadata;
    std::unique_ptr<DomainMetadata> exit_metadata;
  };

  // Types of attributes.
  enum class AttrTy {
    kBool,
    kInt64,
    kInt32,
    kFloat,
    kString,
    kBracedInt64List,
    kBracedInt64ListList,
    kHloComputation,
    kFftType,
    kWindow,
    kConvolutionDimensionNumbers,
    kSharding,
    kInstructionList,
    kSliceRanges,
    kPaddingConfig,
    kMetadata,
    kFusionKind,
    kDistribution,
    kDomain,
    kPrecisionList,
    kShapeList
  };

  struct AttrConfig {
    bool required;     // whether it's required or optional
    AttrTy attr_type;  // what type it is
    void* result;      // where to store the parsed result.
  };

  // attributes ::= (',' attribute)*
  //
  // Parses attributes given names and configs of the attributes. Each parsed
  // result is passed back through the result pointer in corresponding
  // AttrConfig. Note that the result pointer must point to a optional<T> typed
  // variable which outlives this function. Returns false on error. You should
  // not use the any of the results if this function failed.
  //
  // Example usage:
  //
  //  std::unordered_map<string, AttrConfig> attrs;
  //  optional<int64> foo;
  //  attrs["foo"] = {/*required=*/false, AttrTy::kInt64, &foo};
  //  optional<Window> bar;
  //  attrs["bar"] = {/*required=*/true, AttrTy::kWindow, &bar};
  //  if (!ParseAttributes(attrs)) {
  //    return false; // Do not use 'foo' 'bar' if failed.
  //  }
  //  // Do something with 'bar'.
  //  if (foo) { // If attr foo is seen, do something with 'foo'. }
  //
  bool ParseAttributes(const std::unordered_map<string, AttrConfig>& attrs);

  // sub_attributes ::= '{' (','? attribute)* '}'
  //
  // Usage is the same as ParseAttributes. See immediately above.
  bool ParseSubAttributes(const std::unordered_map<string, AttrConfig>& attrs);

  // Parses one attribute. If it has already been seen, return error. Returns
  // true and adds to seen_attrs on success.
  //
  // Do not call this except in ParseAttributes or ParseSubAttributes.
  bool ParseAttributeHelper(const std::unordered_map<string, AttrConfig>& attrs,
                            std::unordered_set<string>* seen_attrs);

  // Parses a name and finds the corresponding hlo computation.
  bool ParseComputationName(HloComputation** value);
  // Parses a list of names and finds the corresponding hlo instructions.
  bool ParseInstructionNames(std::vector<HloInstruction*>* instructions);
  // Pass expect_outer_curlies == true when parsing a Window in the context of a
  // larger computation.  Pass false when parsing a stand-alone Window string.
  bool ParseWindow(Window* window, bool expect_outer_curlies);
  bool ParseConvolutionDimensionNumbers(ConvolutionDimensionNumbers* dnums);
  bool ParsePaddingConfig(PaddingConfig* padding);
  bool ParseMetadata(OpMetadata* metadata);
  bool ParseSharding(OpSharding* sharding);
  bool ParseSingleSharding(OpSharding* sharding, bool lbrace_pre_lexed);

  // Parses the metadata behind a kDOmain instruction.
  bool ParseDomain(DomainData* domain);

  // Parses a sub-attribute of the window attribute, e.g.,size=1x2x3.
  bool ParseDxD(const string& name, std::vector<tensorflow::int64>* result);
  // Parses window's pad sub-attriute, e.g., pad=0_0x3x3.
  bool ParseWindowPad(std::vector<std::vector<tensorflow::int64>>* pad);

  bool ParseSliceRanges(SliceRanges* result);
  bool ParsePrecisionList(std::vector<PrecisionConfig::Precision>* result);
  bool ParseShapeList(std::vector<Shape>* result);
  bool ParseInt64List(const TokKind start, const TokKind end,
                      const TokKind delim,
                      std::vector<tensorflow::int64>* result);
  // 'parse_and_add_item' is an lambda to parse an element in the list and add
  // the parsed element to the result. It's supposed to capture the result.
  bool ParseList(const TokKind start, const TokKind end, const TokKind delim,
                 const std::function<bool()>& parse_and_add_item);

  bool ParseParamListToShape(Shape* shape, LocTy* shape_loc);
  bool ParseParamList();
  bool ParseName(string* result);
  bool ParseAttributeName(string* result);
  bool ParseString(string* result);
  bool ParseShape(Shape* result);
  bool ParseOpcode(HloOpcode* result);
  bool ParseFftType(FftType* result);
  bool ParseFusionKind(HloInstruction::FusionKind* result);
  bool ParseRandomDistribution(RandomDistribution* result);
  bool ParsePrecision(PrecisionConfig::Precision* result);
  bool ParseInt64(tensorflow::int64* result);
  bool ParseDouble(double* result);
  bool ParseBool(bool* result);
  bool ParseToken(TokKind kind, const string& msg);

  // Returns true if the current token is the beginning of a shape.
  bool CanBeShape();
  // Returns true if the current token is the beginning of a
  // param_list_to_shape.
  bool CanBeParamListToShape();

  // Logs the current parsing line and the given message. Always returns false.
  bool TokenError(absl::string_view msg);
  bool Error(LocTy loc, absl::string_view msg);

  // If the current token is 'kind', eats it (i.e. lexes the next token) and
  // returns true.
  bool EatIfPresent(TokKind kind);
  // Parses a shape, and returns true if the result is compatible with the given
  // shape.
  bool EatShapeAndCheckCompatible(const Shape& shape);

  // Adds the instruction to the pool. Returns false and emits an error if the
  // instruction already exists.
  bool AddInstruction(const string& name, HloInstruction* instruction,
                      LocTy name_loc);
  // Adds the computation to the pool. Returns false and emits an error if the
  // computation already exists.
  bool AddComputation(const string& name, HloComputation* computation,
                      LocTy name_loc);

  HloLexer lexer_;

  // A stack for the instruction names. The top of the stack stores the
  // instruction name table for the current scope.
  //
  // A instruction's name is unique among its scope (i.e. its parent
  // computation), but it's not necessarily unique among all computations in the
  // module. When there are multiple levels of nested computations, the same
  // name could appear in both an outer computation and an inner computation. So
  // we need a stack to make sure a name is only visible within its scope,
  std::vector<InstrNameTable> scoped_name_tables_;

  // A helper class which pushes and pops to an InstrNameTable stack via RAII.
  class Scope {
   public:
    explicit Scope(std::vector<InstrNameTable>* scoped_name_tables)
        : scoped_name_tables_(scoped_name_tables) {
      scoped_name_tables_->emplace_back();
    }
    ~Scope() { scoped_name_tables_->pop_back(); }

   private:
    std::vector<InstrNameTable>* scoped_name_tables_;
  };

  // Map from the computation name to the computation itself and its location.
  std::unordered_map<string, std::pair<HloComputation*, LocTy>>
      computation_pool_;

  std::vector<std::unique_ptr<HloComputation>> computations_;
  std::vector<string> error_;

  // When an operand name cannot be resolved, this function is called to create
  // a parameter instruction with the given name and shape. It registers the
  // name, instruction, and a placeholder location in the name table. It returns
  // the newly-created instruction and the placeholder location. If `name` is
  // empty, this should create the parameter with a generated name. This is
  // supposed to be set and used only in ParseSingleInstruction.
  std::function<std::pair<HloInstruction*, LocTy>*(const string& name,
                                                   const Shape& shape)>
      create_missing_instruction_;
};

bool SplitToInt64s(absl::string_view s, char delim, std::vector<int64>* out) {
  for (const auto& split : absl::StrSplit(s, delim)) {
    int64 val;
    if (!absl::SimpleAtoi(split, &val)) {
      return false;
    }
    out->push_back(val);
  }
  return true;
}

// Creates replica groups from the provided nested array. groups[i] represents
// the replica ids for group 'i'.
std::vector<ReplicaGroup> CreateReplicaGroups(
    absl::Span<const std::vector<int64>> groups) {
  std::vector<ReplicaGroup> replica_groups;
  absl::c_transform(groups, std::back_inserter(replica_groups),
                    [](const std::vector<int64>& ids) {
                      ReplicaGroup group;
                      *group.mutable_replica_ids() = {ids.begin(), ids.end()};
                      return group;
                    });
  return replica_groups;
}

bool HloParser::Error(LocTy loc, absl::string_view msg) {
  auto line_col = lexer_.GetLineAndColumn(loc);
  const unsigned line = line_col.first;
  const unsigned col = line_col.second;
  std::vector<string> error_lines;
  error_lines.push_back(
      StrCat("was parsing ", line, ":", col, ": error: ", msg));
  error_lines.emplace_back(lexer_.GetLine(loc));
  error_lines.push_back(col == 0 ? "" : StrCat(string(col - 1, ' '), "^"));

  error_.push_back(StrJoin(error_lines, "\n"));
  VLOG(1) << "Error: " << error_.back();
  return false;
}

bool HloParser::TokenError(absl::string_view msg) {
  return Error(lexer_.GetLoc(), msg);
}

Status HloParser::Run(HloModule* module) {
  lexer_.Lex();
  if (lexer_.GetKind() == TokKind::kw_HloModule) {
    // This means that the text contains a full HLO module.
    if (!ParseHloModule(module)) {
      return InvalidArgument(
          "Syntax error when trying to parse the text as a HloModule:\n%s",
          GetError());
    }
    return Status::OK();
  }
  // This means that the text is a single HLO instruction.
  if (!ParseSingleInstruction(module)) {
    return InvalidArgument(
        "Syntax error when trying to parse the text as a single "
        "HloInstruction:\n%s",
        GetError());
  }
  return Status::OK();
}

std::pair<HloInstruction*, HloParser::LocTy>* HloParser::FindInstruction(
    const string& name, const optional<Shape>& shape) {
  std::pair<HloInstruction*, LocTy>* instr = nullptr;
  if (!name.empty()) {
    instr = tensorflow::gtl::FindOrNull(current_name_table(), name);
  }

  // Potentially call the missing instruction hook.
  if (instr == nullptr && create_missing_instruction_ != nullptr &&
      scoped_name_tables_.size() == 1) {
    if (!shape.has_value()) {
      Error(lexer_.GetLoc(),
            "Operand had no shape in HLO text; cannot create parameter for "
            "single-instruction module.");
      return nullptr;
    }
    return create_missing_instruction_(name, *shape);
  }

  if (instr != nullptr && shape.has_value() &&
      !ShapeUtil::Compatible(instr->first->shape(), shape.value())) {
    Error(
        lexer_.GetLoc(),
        StrCat("The declared operand shape ",
               ShapeUtil::HumanStringWithLayout(shape.value()),
               " is not compatible with the shape of the operand instruction ",
               ShapeUtil::HumanStringWithLayout(instr->first->shape()), "."));
    return nullptr;
  }

  return instr;
}

// ::= 'HloModule' name computations
bool HloParser::ParseHloModule(HloModule* module) {
  if (lexer_.GetKind() != TokKind::kw_HloModule) {
    return TokenError("expects HloModule");
  }
  // Eat 'HloModule'
  lexer_.Lex();

  string name;
  if (!ParseName(&name)) {
    return false;
  }

  absl::optional<bool> is_scheduled;
  std::unordered_map<string, AttrConfig> attrs;
  attrs["is_scheduled"] = {/*required=*/false, AttrTy::kBool, &is_scheduled};
  if (!ParseAttributes(attrs)) {
    return false;
  }

  module->set_name(name);
  if (!ParseComputations(module)) {
    return false;
  }

  if (is_scheduled.has_value() && *is_scheduled) {
    TF_CHECK_OK(module->set_schedule(ScheduleFromInstructionOrder(module)));
  }

  return true;
}

// computations ::= (computation)+
bool HloParser::ParseComputations(HloModule* module) {
  HloComputation* entry_computation = nullptr;
  do {
    if (!ParseComputation(&entry_computation)) {
      return false;
    }
  } while (lexer_.GetKind() != TokKind::kEof);

  for (int i = 0; i < computations_.size(); i++) {
    // If entry_computation is not nullptr, it means the computation it pointed
    // to is marked with "ENTRY"; otherwise, no computation is marked with
    // "ENTRY", and we use the last computation as the entry computation. We
    // add the non-entry computations as embedded computations to the module.
    if ((entry_computation != nullptr &&
         computations_[i].get() != entry_computation) ||
        (entry_computation == nullptr && i != computations_.size() - 1)) {
      module->AddEmbeddedComputation(std::move(computations_[i]));
      continue;
    }
    auto computation = module->AddEntryComputation(std::move(computations_[i]));
    // The parameters and result layouts were set to default layout. Here we
    // set the layouts to what the hlo text says.
    for (int p = 0; p < computation->num_parameters(); p++) {
      const Shape& param_shape = computation->parameter_instruction(p)->shape();
      TF_CHECK_OK(module->mutable_entry_computation_layout()
                      ->mutable_parameter_layout(p)
                      ->CopyLayoutFromShape(param_shape));
    }
    const Shape& result_shape = computation->root_instruction()->shape();
    TF_CHECK_OK(module->mutable_entry_computation_layout()
                    ->mutable_result_layout()
                    ->CopyLayoutFromShape(result_shape));
  }
  return true;
}

// computation ::= ('ENTRY')? name (param_list_to_shape)? instruction_list
bool HloParser::ParseComputation(HloComputation** entry_computation) {
  LocTy maybe_entry_loc = lexer_.GetLoc();
  const bool is_entry_computation = EatIfPresent(TokKind::kw_ENTRY);

  string name;
  LocTy name_loc = lexer_.GetLoc();
  if (!ParseName(&name)) {
    return false;
  }

  LocTy shape_loc = nullptr;
  Shape shape;
  if (CanBeParamListToShape() && !ParseParamListToShape(&shape, &shape_loc)) {
    return false;
  }

  HloComputation* computation = nullptr;
  if (!ParseInstructionList(&computation, name)) {
    return false;
  }

  // If param_list_to_shape was present, check compatibility.
  if (shape_loc != nullptr &&
      !ShapeUtil::Compatible(computation->root_instruction()->shape(), shape)) {
    return Error(
        shape_loc,
        StrCat(
            "Shape of computation ", name, ", ", ShapeUtil::HumanString(shape),
            ", is not compatible with that of its root instruction ",
            computation->root_instruction()->name(), ", ",
            ShapeUtil::HumanString(computation->root_instruction()->shape())));
  }

  if (is_entry_computation) {
    if (*entry_computation != nullptr) {
      return Error(maybe_entry_loc, "expects only one ENTRY");
    }
    *entry_computation = computation;
  }

  return AddComputation(name, computation, name_loc);
}

// instruction_list ::= '{' instruction_list1 '}'
// instruction_list1 ::= (instruction)+
bool HloParser::ParseInstructionList(HloComputation** computation,
                                     const string& computation_name) {
  Scope scope(&scoped_name_tables_);
  HloComputation::Builder builder(computation_name);
  if (!ParseToken(TokKind::kLbrace,
                  "expects '{' at the beginning of instruction list.")) {
    return false;
  }
  string root_name;
  do {
    if (!ParseInstruction(&builder, &root_name)) {
      return false;
    }
  } while (lexer_.GetKind() != TokKind::kRbrace);
  if (!ParseToken(TokKind::kRbrace,
                  "expects '}' at the end of instruction list.")) {
    return false;
  }
  HloInstruction* root = nullptr;
  if (!root_name.empty()) {
    std::pair<HloInstruction*, LocTy>* root_node =
        tensorflow::gtl::FindOrNull(current_name_table(), root_name);

    // This means some instruction was marked as ROOT but we didn't find it in
    // the pool, which should not happen.
    if (root_node == nullptr) {
      LOG(FATAL) << "instruction " << root_name
                 << " was marked as ROOT but the parser has not seen it before";
    }
    root = root_node->first;
  }

  // Now root can be either an existing instruction or a nullptr. If it's a
  // nullptr, the implementation of Builder will set the last instruction as
  // the root instruction.
  computations_.emplace_back(builder.Build(root));
  *computation = computations_.back().get();
  return true;
}

// instruction ::= ('ROOT')? name '=' shape opcode operands (attribute)*
bool HloParser::ParseInstruction(HloComputation::Builder* builder,
                                 string* root_name) {
  string name;
  LocTy maybe_root_loc = lexer_.GetLoc();
  bool is_root = EatIfPresent(TokKind::kw_ROOT);

  const LocTy name_loc = lexer_.GetLoc();
  if (!ParseName(&name) ||
      !ParseToken(TokKind::kEqual, "expects '=' in instruction")) {
    return false;
  }

  if (is_root) {
    if (!root_name->empty()) {
      return Error(maybe_root_loc, "one computation should have only one ROOT");
    }
    *root_name = name;
  }

  return ParseInstructionRhs(builder, name, name_loc);
}

bool HloParser::ParseInstructionRhs(HloComputation::Builder* builder,
                                    const string& name, LocTy name_loc) {
  Shape shape;
  HloOpcode opcode;
  std::vector<HloInstruction*> operands;

  if (!ParseShape(&shape) || !ParseOpcode(&opcode)) {
    return false;
  }

  // Add optional attributes.
  std::unordered_map<string, AttrConfig> attrs;
  optional<OpSharding> sharding;
  attrs["sharding"] = {/*required=*/false, AttrTy::kSharding, &sharding};
  optional<std::vector<HloInstruction*>> predecessors;
  attrs["control-predecessors"] = {/*required=*/false, AttrTy::kInstructionList,
                                   &predecessors};
  optional<OpMetadata> metadata;
  attrs["metadata"] = {/*required=*/false, AttrTy::kMetadata, &metadata};

  optional<string> backend_config;
  attrs["backend_config"] = {/*required=*/false, AttrTy::kString,
                             &backend_config};

  HloInstruction* instruction;
  switch (opcode) {
    case HloOpcode::kParameter: {
      tensorflow::int64 parameter_number;
      if (!ParseToken(TokKind::kLparen,
                      "expects '(' before parameter number") ||
          !ParseInt64(&parameter_number) ||
          !ParseToken(TokKind::kRparen, "expects ')' after parameter number") ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateParameter(parameter_number, shape, name));
      break;
    }
    case HloOpcode::kConstant: {
      Literal literal;
      if (!ParseToken(TokKind::kLparen,
                      "expects '(' before constant literal") ||
          !ParseLiteral(&literal, shape) ||
          !ParseToken(TokKind::kRparen, "expects ')' after constant literal") ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateConstant(std::move(literal)));
      break;
    }
    case HloOpcode::kIota: {
      optional<tensorflow::int64> iota_dimension;
      attrs["iota_dimension"] = {/*required=*/true, AttrTy::kInt64,
                                 &iota_dimension};
      if (!ParseOperands(&operands, /*expected_size=*/0) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateIota(shape, *iota_dimension));
      break;
    }
    // Unary ops.
    case HloOpcode::kAbs:
    case HloOpcode::kRoundNearestAfz:
    case HloOpcode::kBitcast:
    case HloOpcode::kCeil:
    case HloOpcode::kClz:
    case HloOpcode::kCopy:
    case HloOpcode::kCos:
    case HloOpcode::kExp:
    case HloOpcode::kExpm1:
    case HloOpcode::kImag:
    case HloOpcode::kIsFinite:
    case HloOpcode::kFloor:
    case HloOpcode::kLog:
    case HloOpcode::kLog1p:
    case HloOpcode::kNot:
    case HloOpcode::kNegate:
    case HloOpcode::kReal:
    case HloOpcode::kSign:
    case HloOpcode::kSin:
    case HloOpcode::kTanh: {
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateUnary(shape, opcode, operands[0]));
      break;
    }
    // Binary ops.
    case HloOpcode::kAdd:
    case HloOpcode::kDivide:
    case HloOpcode::kMultiply:
    case HloOpcode::kSubtract:
    case HloOpcode::kAtan2:
    case HloOpcode::kComplex:
    case HloOpcode::kEq:
    case HloOpcode::kGe:
    case HloOpcode::kGt:
    case HloOpcode::kLe:
    case HloOpcode::kLt:
    case HloOpcode::kNe:
    case HloOpcode::kMaximum:
    case HloOpcode::kMinimum:
    case HloOpcode::kPower:
    case HloOpcode::kRemainder:
    case HloOpcode::kAnd:
    case HloOpcode::kOr:
    case HloOpcode::kXor:
    case HloOpcode::kShiftLeft:
    case HloOpcode::kShiftRightArithmetic:
    case HloOpcode::kShiftRightLogical: {
      if (!ParseOperands(&operands, /*expected_size=*/2) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateBinary(
          shape, opcode, operands[0], operands[1]));
      break;
    }
    // Ternary ops.
    case HloOpcode::kClamp:
    case HloOpcode::kSelect:
    case HloOpcode::kTupleSelect: {
      if (!ParseOperands(&operands, /*expected_size=*/3) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateTernary(
          shape, opcode, operands[0], operands[1], operands[2]));
      break;
    }
    // Other supported ops.
    case HloOpcode::kConvert: {
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateConvert(shape, operands[0]));
      break;
    }
    case HloOpcode::kBitcastConvert: {
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateBitcastConvert(shape, operands[0]));
      break;
    }
    case HloOpcode::kCrossReplicaSum: {
      optional<std::vector<std::vector<int64>>> tmp_groups;
      optional<HloComputation*> to_apply;
      optional<std::vector<int64>> replica_group_ids;
      optional<string> barrier;
      optional<int64> all_reduce_id;
      attrs["to_apply"] = {/*required=*/true, AttrTy::kHloComputation,
                           &to_apply};
      attrs["replica_groups"] = {/*required=*/false,
                                 AttrTy::kBracedInt64ListList, &tmp_groups};
      attrs["barrier"] = {/*required=*/false, AttrTy::kString, &barrier};
      attrs["all_reduce_id"] = {/*required=*/false, AttrTy::kInt64,
                                &all_reduce_id};
      if (!ParseOperands(&operands) || !ParseAttributes(attrs)) {
        return false;
      }
      std::vector<ReplicaGroup> replica_groups;
      if (tmp_groups) {
        replica_groups = CreateReplicaGroups(*tmp_groups);
      }
      instruction =
          builder->AddInstruction(HloInstruction::CreateCrossReplicaSum(
              shape, operands, *to_apply, replica_groups,
              barrier ? *barrier : "", all_reduce_id));
      break;
    }
    case HloOpcode::kAllToAll: {
      optional<std::vector<std::vector<int64>>> tmp_groups;
      optional<string> barrier;
      attrs["replica_groups"] = {/*required=*/false,
                                 AttrTy::kBracedInt64ListList, &tmp_groups};
      if (!ParseOperands(&operands) || !ParseAttributes(attrs)) {
        return false;
      }
      std::vector<ReplicaGroup> replica_groups;
      if (tmp_groups) {
        replica_groups = CreateReplicaGroups(*tmp_groups);
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateAllToAll(shape, operands, replica_groups));
      break;
    }
    case HloOpcode::kCollectivePermute: {
      optional<std::vector<std::vector<int64>>> source_targets;
      attrs["source_target_pairs"] = {
          /*required=*/true, AttrTy::kBracedInt64ListList, &source_targets};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      std::vector<std::pair<int64, int64>> pairs(source_targets->size());
      for (int i = 0; i < pairs.size(); i++) {
        if ((*source_targets)[i].size() != 2) {
          return TokenError(
              "expects 'source_target_pairs=' to be a list of pairs");
        }
        pairs[i].first = (*source_targets)[i][0];
        pairs[i].second = (*source_targets)[i][1];
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateCollectivePermute(shape, operands[0], pairs));
      break;
    }
    case HloOpcode::kReshape: {
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateReshape(shape, operands[0]));
      break;
    }
    case HloOpcode::kAfterAll: {
      if (!ParseOperands(&operands) || !ParseAttributes(attrs)) {
        return false;
      }
      if (operands.empty()) {
        instruction = builder->AddInstruction(HloInstruction::CreateToken());
      } else {
        instruction =
            builder->AddInstruction(HloInstruction::CreateAfterAll(operands));
      }
      break;
    }
    case HloOpcode::kAddDependency: {
      if (!ParseOperands(&operands, /*expected_size=*/2) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateAddDependency(operands[0], operands[1]));
      break;
    }
    case HloOpcode::kSort: {
      optional<std::vector<tensorflow::int64>> dimensions;
      attrs["dimensions"] = {/*required=*/true, AttrTy::kBracedInt64List,
                             &dimensions};
      if (!ParseOperands(&operands) || !ParseAttributes(attrs) ||
          dimensions->size() != 1) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateSort(
          shape, dimensions->at(0),
          /*keys=*/operands[0],
          /*values=*/absl::Span<HloInstruction* const>(operands).subspan(1)));
      break;
    }
    case HloOpcode::kTuple: {
      if (!ParseOperands(&operands) || !ParseAttributes(attrs)) {
        return false;
      }
      instruction =
          builder->AddInstruction(HloInstruction::CreateTuple(operands));
      break;
    }
    case HloOpcode::kWhile: {
      optional<HloComputation*> condition;
      optional<HloComputation*> body;
      attrs["condition"] = {/*required=*/true, AttrTy::kHloComputation,
                            &condition};
      attrs["body"] = {/*required=*/true, AttrTy::kHloComputation, &body};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateWhile(
          shape, *condition, *body, /*init=*/operands[0]));
      break;
    }
    case HloOpcode::kRecv: {
      optional<tensorflow::int64> channel_id;
      // If the is_host_transfer attribute is not present then default to false.
      optional<bool> is_host_transfer = false;
      attrs["channel_id"] = {/*required=*/true, AttrTy::kInt64, &channel_id};
      attrs["is_host_transfer"] = {/*required=*/false, AttrTy::kBool,
                                   &is_host_transfer};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      // If the is_host_transfer attribute is not present then default to false.
      instruction = builder->AddInstruction(HloInstruction::CreateRecv(
          shape.tuple_shapes(0), operands[0], *channel_id, *is_host_transfer));
      break;
    }
    case HloOpcode::kRecvDone: {
      optional<tensorflow::int64> channel_id;
      // If the is_host_transfer attribute is not present then default to false.
      optional<bool> is_host_transfer = false;
      attrs["channel_id"] = {/*required=*/true, AttrTy::kInt64, &channel_id};
      attrs["is_host_transfer"] = {/*required=*/false, AttrTy::kBool,
                                   &is_host_transfer};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      if (channel_id != operands[0]->channel_id()) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateRecvDone(operands[0], *is_host_transfer));
      break;
    }
    case HloOpcode::kSend: {
      optional<tensorflow::int64> channel_id;
      // If the is_host_transfer attribute is not present then default to false.
      optional<bool> is_host_transfer = false;
      attrs["channel_id"] = {/*required=*/true, AttrTy::kInt64, &channel_id};
      attrs["is_host_transfer"] = {/*required=*/false, AttrTy::kBool,
                                   &is_host_transfer};
      if (!ParseOperands(&operands, /*expected_size=*/2) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateSend(
          operands[0], operands[1], *channel_id, *is_host_transfer));
      break;
    }
    case HloOpcode::kSendDone: {
      optional<tensorflow::int64> channel_id;
      // If the is_host_transfer attribute is not present then default to false.
      optional<bool> is_host_transfer = false;
      attrs["channel_id"] = {/*required=*/true, AttrTy::kInt64, &channel_id};
      attrs["is_host_transfer"] = {/*required=*/false, AttrTy::kBool,
                                   &is_host_transfer};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      if (channel_id != operands[0]->channel_id()) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateSendDone(operands[0], *is_host_transfer));
      break;
    }
    case HloOpcode::kGetTupleElement: {
      optional<tensorflow::int64> index;
      attrs["index"] = {/*required=*/true, AttrTy::kInt64, &index};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateGetTupleElement(shape, operands[0], *index));
      break;
    }
    case HloOpcode::kCall: {
      optional<HloComputation*> to_apply;
      attrs["to_apply"] = {/*required=*/true, AttrTy::kHloComputation,
                           &to_apply};
      if (!ParseOperands(&operands) || !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateCall(shape, operands, *to_apply));
      break;
    }
    case HloOpcode::kReduceWindow: {
      optional<HloComputation*> reduce_computation;
      optional<Window> window;
      attrs["window"] = {/*required=*/false, AttrTy::kWindow, &window};
      attrs["to_apply"] = {/*required=*/true, AttrTy::kHloComputation,
                           &reduce_computation};
      if (!ParseOperands(&operands, /*expected_size=*/2) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      if (!window) {
        window.emplace();
      }
      instruction = builder->AddInstruction(HloInstruction::CreateReduceWindow(
          shape, /*operand=*/operands[0], /*init_value=*/operands[1], *window,
          *reduce_computation));
      break;
    }
    case HloOpcode::kConvolution: {
      optional<Window> window;
      optional<ConvolutionDimensionNumbers> dnums;
      optional<int64> feature_group_count;
      attrs["window"] = {/*required=*/false, AttrTy::kWindow, &window};
      attrs["dim_labels"] = {/*required=*/true,
                             AttrTy::kConvolutionDimensionNumbers, &dnums};
      attrs["feature_group_count"] = {/*required=*/false, AttrTy::kInt64,
                                      &feature_group_count};
      optional<std::vector<PrecisionConfig::Precision>> operand_precision;
      attrs["operand_precision"] = {/*required=*/false, AttrTy::kPrecisionList,
                                    &operand_precision};
      if (!ParseOperands(&operands, /*expected_size=*/2) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      if (!window) {
        window.emplace();
      }
      if (!feature_group_count) {
        feature_group_count = 1;
      }
      PrecisionConfig precision_config;
      if (operand_precision) {
        *precision_config.mutable_operand_precision() = {
            operand_precision->begin(), operand_precision->end()};
      } else {
        precision_config.mutable_operand_precision()->Resize(
            operands.size(), PrecisionConfig::DEFAULT);
      }
      instruction = builder->AddInstruction(HloInstruction::CreateConvolve(
          shape, /*lhs=*/operands[0], /*rhs=*/operands[1],
          feature_group_count.value(), *window, *dnums, precision_config));
      break;
    }
    case HloOpcode::kFft: {
      optional<FftType> fft_type;
      optional<std::vector<tensorflow::int64>> fft_length;
      attrs["fft_type"] = {/*required=*/true, AttrTy::kFftType, &fft_type};
      attrs["fft_length"] = {/*required=*/true, AttrTy::kBracedInt64List,
                             &fft_length};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateFft(
          shape, operands[0], *fft_type, *fft_length));
      break;
    }
    case HloOpcode::kBroadcast: {
      optional<std::vector<tensorflow::int64>> broadcast_dimensions;
      attrs["dimensions"] = {/*required=*/true, AttrTy::kBracedInt64List,
                             &broadcast_dimensions};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateBroadcast(
          shape, operands[0], *broadcast_dimensions));
      break;
    }
    case HloOpcode::kConcatenate: {
      optional<std::vector<tensorflow::int64>> dimensions;
      attrs["dimensions"] = {/*required=*/true, AttrTy::kBracedInt64List,
                             &dimensions};
      if (!ParseOperands(&operands) || !ParseAttributes(attrs) ||
          dimensions->size() != 1) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateConcatenate(
          shape, operands, dimensions->at(0)));
      break;
    }
    case HloOpcode::kMap: {
      optional<HloComputation*> to_apply;
      attrs["to_apply"] = {/*required=*/true, AttrTy::kHloComputation,
                           &to_apply};
      optional<std::vector<tensorflow::int64>> dimensions;
      attrs["dimensions"] = {/*required=*/false, AttrTy::kBracedInt64List,
                             &dimensions};
      if (!ParseOperands(&operands) || !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateMap(shape, operands, *to_apply));
      break;
    }
    case HloOpcode::kReduce: {
      auto loc = lexer_.GetLoc();

      optional<HloComputation*> reduce_computation;
      attrs["to_apply"] = {/*required=*/true, AttrTy::kHloComputation,
                           &reduce_computation};
      optional<std::vector<tensorflow::int64>> dimensions_to_reduce;
      attrs["dimensions"] = {/*required=*/true, AttrTy::kBracedInt64List,
                             &dimensions_to_reduce};
      if (!ParseOperands(&operands) || !ParseAttributes(attrs)) {
        return false;
      }
      if (operands.size() % 2) {
        return Error(loc, StrCat("expects an even number of operands, but has ",
                                 operands.size(), " operands"));
      }
      instruction = builder->AddInstruction(HloInstruction::CreateReduce(
          shape, /*operands=*/
          absl::Span<HloInstruction* const>(operands).subspan(
              0, operands.size() / 2),
          /*init_values=*/
          absl::Span<HloInstruction* const>(operands).subspan(operands.size() /
                                                              2),
          *dimensions_to_reduce, *reduce_computation));
      break;
    }
    case HloOpcode::kReverse: {
      optional<std::vector<tensorflow::int64>> dimensions;
      attrs["dimensions"] = {/*required=*/true, AttrTy::kBracedInt64List,
                             &dimensions};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateReverse(shape, operands[0], *dimensions));
      break;
    }
    case HloOpcode::kSelectAndScatter: {
      optional<HloComputation*> select;
      attrs["select"] = {/*required=*/true, AttrTy::kHloComputation, &select};
      optional<HloComputation*> scatter;
      attrs["scatter"] = {/*required=*/true, AttrTy::kHloComputation, &scatter};
      optional<Window> window;
      attrs["window"] = {/*required=*/false, AttrTy::kWindow, &window};
      if (!ParseOperands(&operands, /*expected_size=*/3) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      if (!window) {
        window.emplace();
      }
      instruction =
          builder->AddInstruction(HloInstruction::CreateSelectAndScatter(
              shape, /*operand=*/operands[0], *select, *window,
              /*source=*/operands[1], /*init_value=*/operands[2], *scatter));
      break;
    }
    case HloOpcode::kSlice: {
      optional<SliceRanges> slice_ranges;
      attrs["slice"] = {/*required=*/true, AttrTy::kSliceRanges, &slice_ranges};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateSlice(
          shape, operands[0], slice_ranges->starts, slice_ranges->limits,
          slice_ranges->strides));
      break;
    }
    case HloOpcode::kDynamicSlice: {
      optional<std::vector<tensorflow::int64>> dynamic_slice_sizes;
      attrs["dynamic_slice_sizes"] = {
          /*required=*/true, AttrTy::kBracedInt64List, &dynamic_slice_sizes};
      if (!ParseOperands(&operands, /*expected_size=*/2) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateDynamicSlice(
          shape, /*operand=*/operands[0], /*start_indices=*/operands[1],
          *dynamic_slice_sizes));
      break;
    }
    case HloOpcode::kDynamicUpdateSlice: {
      if (!ParseOperands(&operands, /*expected_size=*/3) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction =
          builder->AddInstruction(HloInstruction::CreateDynamicUpdateSlice(
              shape, /*operand=*/operands[0], /*update=*/operands[1],
              /*start_indices=*/operands[2]));
      break;
    }
    case HloOpcode::kTranspose: {
      optional<std::vector<tensorflow::int64>> dimensions;
      attrs["dimensions"] = {/*required=*/true, AttrTy::kBracedInt64List,
                             &dimensions};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateTranspose(shape, operands[0], *dimensions));
      break;
    }
    case HloOpcode::kBatchNormTraining: {
      optional<float> epsilon;
      attrs["epsilon"] = {/*required=*/true, AttrTy::kFloat, &epsilon};
      optional<tensorflow::int64> feature_index;
      attrs["feature_index"] = {/*required=*/true, AttrTy::kInt64,
                                &feature_index};
      if (!ParseOperands(&operands, /*expected_size=*/3) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction =
          builder->AddInstruction(HloInstruction::CreateBatchNormTraining(
              shape, /*operand=*/operands[0], /*scale=*/operands[1],
              /*offset=*/operands[2], *epsilon, *feature_index));
      break;
    }
    case HloOpcode::kBatchNormInference: {
      optional<float> epsilon;
      attrs["epsilon"] = {/*required=*/true, AttrTy::kFloat, &epsilon};
      optional<tensorflow::int64> feature_index;
      attrs["feature_index"] = {/*required=*/true, AttrTy::kInt64,
                                &feature_index};
      if (!ParseOperands(&operands, /*expected_size=*/5) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction =
          builder->AddInstruction(HloInstruction::CreateBatchNormInference(
              shape, /*operand=*/operands[0], /*scale=*/operands[1],
              /*offset=*/operands[2], /*mean=*/operands[3],
              /*variance=*/operands[4], *epsilon, *feature_index));
      break;
    }
    case HloOpcode::kBatchNormGrad: {
      optional<float> epsilon;
      attrs["epsilon"] = {/*required=*/true, AttrTy::kFloat, &epsilon};
      optional<tensorflow::int64> feature_index;
      attrs["feature_index"] = {/*required=*/true, AttrTy::kInt64,
                                &feature_index};
      if (!ParseOperands(&operands, /*expected_size=*/5) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateBatchNormGrad(
          shape, /*operand=*/operands[0], /*scale=*/operands[1],
          /*mean=*/operands[2], /*variance=*/operands[3],
          /*grad_output=*/operands[4], *epsilon, *feature_index));
      break;
    }
    case HloOpcode::kPad: {
      optional<PaddingConfig> padding;
      attrs["padding"] = {/*required=*/true, AttrTy::kPaddingConfig, &padding};
      if (!ParseOperands(&operands, /*expected_size=*/2) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreatePad(
          shape, operands[0], /*padding_value=*/operands[1], *padding));
      break;
    }
    case HloOpcode::kFusion: {
      optional<HloComputation*> fusion_computation;
      attrs["calls"] = {/*required=*/true, AttrTy::kHloComputation,
                        &fusion_computation};
      optional<HloInstruction::FusionKind> fusion_kind;
      attrs["kind"] = {/*required=*/true, AttrTy::kFusionKind, &fusion_kind};
      if (!ParseOperands(&operands) || !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateFusion(
          shape, *fusion_kind, operands, *fusion_computation));
      break;
    }
    case HloOpcode::kInfeed: {
      optional<string> config;
      attrs["infeed_config"] = {/*required=*/false, AttrTy::kString, &config};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      // We need to know the infeed data shape to construct the infeed
      // instruction. This is the zero-th element of the tuple-shaped output of
      // the infeed instruction. ShapeUtil::GetTupleElementShape will check fail
      // if the shape is not a non-empty tuple, so add guard so an error message
      // can be emitted instead of a check fail
      if (!ShapeUtil::IsTuple(shape) && !ShapeUtil::IsEmptyTuple(shape)) {
        return Error(lexer_.GetLoc(),
                     "infeed must have a non-empty tuple shape");
      }
      instruction = builder->AddInstruction(HloInstruction::CreateInfeed(
          ShapeUtil::GetTupleElementShape(shape, 0), operands[0],
          config ? *config : ""));
      break;
    }
    case HloOpcode::kOutfeed: {
      optional<string> config;
      attrs["outfeed_config"] = {/*required=*/false, AttrTy::kString, &config};
      if (!ParseOperands(&operands, /*expected_size=*/2) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateOutfeed(operands[0]->shape(), operands[0],
                                        operands[1], config ? *config : ""));
      break;
    }
    case HloOpcode::kRng: {
      optional<RandomDistribution> distribution;
      attrs["distribution"] = {/*required=*/true, AttrTy::kDistribution,
                               &distribution};
      if (!ParseOperands(&operands) || !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(
          HloInstruction::CreateRng(shape, *distribution, operands));
      break;
    }
    case HloOpcode::kReducePrecision: {
      optional<tensorflow::int64> exponent_bits;
      optional<tensorflow::int64> mantissa_bits;
      attrs["exponent_bits"] = {/*required=*/true, AttrTy::kInt64,
                                &exponent_bits};
      attrs["mantissa_bits"] = {/*required=*/true, AttrTy::kInt64,
                                &mantissa_bits};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction =
          builder->AddInstruction(HloInstruction::CreateReducePrecision(
              shape, operands[0], static_cast<int>(*exponent_bits),
              static_cast<int>(*mantissa_bits)));
      break;
    }
    case HloOpcode::kConditional: {
      optional<HloComputation*> true_computation;
      optional<HloComputation*> false_computation;
      attrs["true_computation"] = {/*required=*/true, AttrTy::kHloComputation,
                                   &true_computation};
      attrs["false_computation"] = {/*required=*/true, AttrTy::kHloComputation,
                                    &false_computation};
      if (!ParseOperands(&operands, /*expected_size=*/3) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateConditional(
          shape, /*pred=*/operands[0],
          /*true_computation_arg=*/operands[1], *true_computation,
          /*false_computation_arg=*/operands[2], *false_computation));
      break;
    }
    case HloOpcode::kCustomCall: {
      optional<string> custom_call_target;
      optional<string> opaque;
      optional<Window> window;
      optional<ConvolutionDimensionNumbers> dnums;
      optional<int64> feature_group_count;
      optional<std::vector<Shape>> operand_layout_constraints;
      attrs["custom_call_target"] = {/*required=*/true, AttrTy::kString,
                                     &custom_call_target};
      attrs["opaque"] = {/*required=*/false, AttrTy::kString, &opaque};
      attrs["window"] = {/*required=*/false, AttrTy::kWindow, &window};
      attrs["dim_labels"] = {/*required=*/false,
                             AttrTy::kConvolutionDimensionNumbers, &dnums};
      attrs["feature_group_count"] = {/*required=*/false, AttrTy::kInt64,
                                      &feature_group_count};
      attrs["operand_layout_constraints"] = {
          /*required=*/false, AttrTy::kShapeList, &operand_layout_constraints};
      if (!ParseOperands(&operands) || !ParseAttributes(attrs)) {
        return false;
      }
      if (operand_layout_constraints.has_value()) {
        if (!LayoutUtil::HasLayout(shape)) {
          return Error(lexer_.GetLoc(),
                       "Layout must be set on layout-constrained custom call");
        }
        if (operands.size() != operand_layout_constraints->size()) {
          return Error(lexer_.GetLoc(),
                       StrCat("Expected ", operands.size(),
                              " operand layout constraints, ",
                              operand_layout_constraints->size(), " given"));
        }
        for (int64 i = 0; i < operands.size(); ++i) {
          const Shape& operand_shape_with_layout =
              (*operand_layout_constraints)[i];
          if (!LayoutUtil::HasLayout(operand_shape_with_layout)) {
            return Error(lexer_.GetLoc(),
                         StrCat("Operand layout constraint shape ",
                                ShapeUtil::HumanStringWithLayout(
                                    operand_shape_with_layout),
                                " for operand ", i, " does not have a layout"));
          }
          if (!ShapeUtil::Compatible(operand_shape_with_layout,
                                     operands[i]->shape())) {
            return Error(
                lexer_.GetLoc(),
                StrCat(
                    "Operand layout constraint shape ",
                    ShapeUtil::HumanStringWithLayout(operand_shape_with_layout),
                    " for operand ", i,
                    " is not compatible with operand shape ",
                    ShapeUtil::HumanStringWithLayout(operands[i]->shape())));
          }
        }
        instruction = builder->AddInstruction(HloInstruction::CreateCustomCall(
            shape, operands, *custom_call_target, *operand_layout_constraints,
            opaque.has_value() ? *opaque : ""));
      } else {
        instruction = builder->AddInstruction(HloInstruction::CreateCustomCall(
            shape, operands, *custom_call_target,
            opaque.has_value() ? *opaque : ""));
      }
      if (window.has_value()) {
        instruction->set_window(*window);
      }
      if (dnums.has_value()) {
        instruction->set_convolution_dimension_numbers(*dnums);
      }
      if (feature_group_count.has_value()) {
        instruction->set_feature_group_count(*feature_group_count);
      }
      break;
    }
    case HloOpcode::kDot: {
      optional<std::vector<tensorflow::int64>> lhs_contracting_dims;
      attrs["lhs_contracting_dims"] = {
          /*required=*/false, AttrTy::kBracedInt64List, &lhs_contracting_dims};
      optional<std::vector<tensorflow::int64>> rhs_contracting_dims;
      attrs["rhs_contracting_dims"] = {
          /*required=*/false, AttrTy::kBracedInt64List, &rhs_contracting_dims};
      optional<std::vector<tensorflow::int64>> lhs_batch_dims;
      attrs["lhs_batch_dims"] = {/*required=*/false, AttrTy::kBracedInt64List,
                                 &lhs_batch_dims};
      optional<std::vector<tensorflow::int64>> rhs_batch_dims;
      attrs["rhs_batch_dims"] = {/*required=*/false, AttrTy::kBracedInt64List,
                                 &rhs_batch_dims};
      optional<std::vector<PrecisionConfig::Precision>> operand_precision;
      attrs["operand_precision"] = {/*required=*/false, AttrTy::kPrecisionList,
                                    &operand_precision};

      if (!ParseOperands(&operands, /*expected_size=*/2) ||
          !ParseAttributes(attrs)) {
        return false;
      }

      DotDimensionNumbers dnum;
      if (lhs_contracting_dims) {
        *dnum.mutable_lhs_contracting_dimensions() = {
            lhs_contracting_dims->begin(), lhs_contracting_dims->end()};
      }
      if (rhs_contracting_dims) {
        *dnum.mutable_rhs_contracting_dimensions() = {
            rhs_contracting_dims->begin(), rhs_contracting_dims->end()};
      }
      if (lhs_batch_dims) {
        *dnum.mutable_lhs_batch_dimensions() = {lhs_batch_dims->begin(),
                                                lhs_batch_dims->end()};
      }
      if (rhs_batch_dims) {
        *dnum.mutable_rhs_batch_dimensions() = {rhs_batch_dims->begin(),
                                                rhs_batch_dims->end()};
      }

      PrecisionConfig precision_config;
      if (operand_precision) {
        *precision_config.mutable_operand_precision() = {
            operand_precision->begin(), operand_precision->end()};
      } else {
        precision_config.mutable_operand_precision()->Resize(
            operands.size(), PrecisionConfig::DEFAULT);
      }

      instruction = builder->AddInstruction(HloInstruction::CreateDot(
          shape, operands[0], operands[1], dnum, precision_config));
      break;
    }
    case HloOpcode::kGather: {
      optional<std::vector<tensorflow::int64>> offset_dims;
      attrs["offset_dims"] = {/*required=*/true, AttrTy::kBracedInt64List,
                              &offset_dims};
      optional<std::vector<tensorflow::int64>> collapsed_slice_dims;
      attrs["collapsed_slice_dims"] = {
          /*required=*/true, AttrTy::kBracedInt64List, &collapsed_slice_dims};
      optional<std::vector<tensorflow::int64>> start_index_map;
      attrs["start_index_map"] = {/*required=*/true, AttrTy::kBracedInt64List,
                                  &start_index_map};
      optional<tensorflow::int64> index_vector_dim;
      attrs["index_vector_dim"] = {/*required=*/true, AttrTy::kInt64,
                                   &index_vector_dim};
      optional<std::vector<tensorflow::int64>> slice_sizes;
      attrs["slice_sizes"] = {/*required=*/true, AttrTy::kBracedInt64List,
                              &slice_sizes};

      if (!ParseOperands(&operands, /*expected_size=*/2) ||
          !ParseAttributes(attrs)) {
        return false;
      }

      GatherDimensionNumbers dim_numbers =
          HloGatherInstruction::MakeGatherDimNumbers(
              /*offset_dims=*/*offset_dims,
              /*collapsed_slice_dims=*/*collapsed_slice_dims,
              /*start_index_map=*/*start_index_map,
              /*index_vector_dim=*/*index_vector_dim);

      instruction = builder->AddInstruction(HloInstruction::CreateGather(
          shape, /*operand=*/operands[0], /*start_indices=*/operands[1],
          dim_numbers, *slice_sizes));
      break;
    }
    case HloOpcode::kScatter: {
      optional<std::vector<tensorflow::int64>> update_window_dims;
      attrs["update_window_dims"] = {
          /*required=*/true, AttrTy::kBracedInt64List, &update_window_dims};
      optional<std::vector<tensorflow::int64>> inserted_window_dims;
      attrs["inserted_window_dims"] = {
          /*required=*/true, AttrTy::kBracedInt64List, &inserted_window_dims};
      optional<std::vector<tensorflow::int64>> scatter_dims_to_operand_dims;
      attrs["scatter_dims_to_operand_dims"] = {/*required=*/true,
                                               AttrTy::kBracedInt64List,
                                               &scatter_dims_to_operand_dims};
      optional<tensorflow::int64> index_vector_dim;
      attrs["index_vector_dim"] = {/*required=*/true, AttrTy::kInt64,
                                   &index_vector_dim};

      optional<HloComputation*> update_computation;
      attrs["to_apply"] = {/*required=*/true, AttrTy::kHloComputation,
                           &update_computation};

      if (!ParseOperands(&operands, /*expected_size=*/3) ||
          !ParseAttributes(attrs)) {
        return false;
      }

      ScatterDimensionNumbers dim_numbers =
          HloScatterInstruction::MakeScatterDimNumbers(
              /*update_window_dims=*/*update_window_dims,
              /*inserted_window_dims=*/*inserted_window_dims,
              /*scatter_dims_to_operand_dims=*/*scatter_dims_to_operand_dims,
              /*index_vector_dim=*/*index_vector_dim);

      instruction = builder->AddInstruction(HloInstruction::CreateScatter(
          shape, /*operand=*/operands[0], /*scatter_indices=*/operands[1],
          /*updates=*/operands[2], *update_computation, dim_numbers));
      break;
    }
    case HloOpcode::kDomain: {
      DomainData domain;
      attrs["domain"] = {/*required=*/true, AttrTy::kDomain, &domain};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction = builder->AddInstruction(HloInstruction::CreateDomain(
          shape, operands[0], std::move(domain.exit_metadata),
          std::move(domain.entry_metadata)));
      break;
    }
    case HloOpcode::kTrace:
      return TokenError(StrCat("parsing not yet implemented for op: ",
                               HloOpcodeString(opcode)));
    case HloOpcode::kGetDimensionSize:
      optional<std::vector<tensorflow::int64>> dimensions;
      attrs["dimensions"] = {/*required=*/true, AttrTy::kBracedInt64List,
                             &dimensions};
      if (!ParseOperands(&operands, /*expected_size=*/1) ||
          !ParseAttributes(attrs)) {
        return false;
      }
      instruction =
          builder->AddInstruction(HloInstruction::CreateGetDimensionSize(
              shape, operands[0], (*dimensions)[0]));
      break;
  }

  instruction->SetAndSanitizeName(name);
  if (instruction->name() != name) {
    return Error(name_loc,
                 StrCat("illegal instruction name: ", name,
                        "; suggest renaming to: ", instruction->name()));
  }

  // Add shared attributes like metadata to the instruction, if they were seen.
  if (sharding) {
    instruction->set_sharding(
        HloSharding::FromProto(sharding.value()).ValueOrDie());
  }
  if (predecessors) {
    for (auto* pre : *predecessors) {
      Status status = pre->AddControlDependencyTo(instruction);
      if (!status.ok()) {
        return Error(name_loc, StrCat("error adding control dependency for: ",
                                      name, " status: ", status.ToString()));
      }
    }
  }
  if (metadata) {
    instruction->set_metadata(*metadata);
  }
  if (backend_config) {
    instruction->set_raw_backend_config_string(std::move(*backend_config));
  }
  return AddInstruction(name, instruction, name_loc);
}  // NOLINT(readability/fn_size)

// ::= '{' (single_sharding | tuple_sharding) '}'
//
// tuple_sharding ::= single_sharding* (',' single_sharding)*
bool HloParser::ParseSharding(OpSharding* sharding) {
  // A single sharding starts with '{' and is not followed by '{'.
  // A tuple sharding starts with '{' and is followed by '{', or is '{''}' for
  // an empty tuple.
  if (!ParseToken(TokKind::kLbrace,
                  "expected '{' to start sharding attribute")) {
    return false;
  }

  if (lexer_.GetKind() != TokKind::kLbrace &&
      lexer_.GetKind() != TokKind::kRbrace) {
    return ParseSingleSharding(sharding, /*lbrace_pre_lexed=*/true);
  }

  // Tuple sharding.
  // Allow empty tuple shardings.
  if (lexer_.GetKind() != TokKind::kRbrace) {
    do {
      if (!ParseSingleSharding(sharding->add_tuple_shardings(),
                               /*lbrace_pre_lexed=*/false)) {
        return false;
      }
    } while (EatIfPresent(TokKind::kComma));
  }
  sharding->set_type(OpSharding::Type::OpSharding_Type_TUPLE);

  return ParseToken(TokKind::kRbrace, "expected '}' to end sharding attribute");
}

//  ::= '{' 'replicated'? 'maximal'? ('device=' int)? shape?
//          ('devices=' ('[' dims ']')* device_list)? '}'
// dims ::= int_list device_list ::= int_list
bool HloParser::ParseSingleSharding(OpSharding* sharding,
                                    bool lbrace_pre_lexed) {
  if (!lbrace_pre_lexed &&
      !ParseToken(TokKind::kLbrace,
                  "expected '{' to start sharding attribute")) {
    return false;
  }

  LocTy loc = lexer_.GetLoc();
  bool maximal = false;
  bool replicated = false;
  std::vector<tensorflow::int64> devices;
  std::vector<tensorflow::int64> tile_assignment_dimensions;
  while (lexer_.GetKind() != TokKind::kRbrace) {
    switch (lexer_.GetKind()) {
      case TokKind::kw_maximal:
        maximal = true;
        lexer_.Lex();
        break;
      case TokKind::kw_replicated:
        replicated = true;
        lexer_.Lex();
        break;
      case TokKind::kAttributeName: {
        if (lexer_.GetStrVal() == "device") {
          if (lexer_.Lex() != TokKind::kInt) {
            return TokenError("device= attribute must be an integer");
          }
          devices = {lexer_.GetInt64Val()};
          lexer_.Lex();
        } else if (lexer_.GetStrVal() == "devices") {
          lexer_.Lex();
          if (!ParseToken(TokKind::kLsquare,
                          "expected '[' to start sharding devices shape")) {
            return false;
          }

          do {
            tensorflow::int64 dim;
            if (!ParseInt64(&dim)) {
              return false;
            }
            tile_assignment_dimensions.push_back(dim);
          } while (EatIfPresent(TokKind::kComma));

          if (!ParseToken(TokKind::kRsquare,
                          "expected ']' to start sharding devices shape")) {
            return false;
          }
          do {
            tensorflow::int64 device;
            if (!ParseInt64(&device)) {
              return false;
            }
            devices.push_back(device);
          } while (EatIfPresent(TokKind::kComma));
        } else {
          return TokenError(
              "unknown attribute in sharding: expected device= or devices=");
        }
        break;
      }
      case TokKind::kShape:
        // TODO(b/112302613): Left here for backward compatibility to ignore the
        // removed tile shape data.
        lexer_.Lex();
        break;
      case TokKind::kRbrace:
        break;
      default:
        return TokenError("unexpected token");
    }
  }

  if (replicated) {
    if (!devices.empty()) {
      return Error(loc,
                   "replicated shardings should not have any devices assigned");
    }
    sharding->set_type(OpSharding::Type::OpSharding_Type_REPLICATED);
  } else if (maximal) {
    if (devices.size() != 1) {
      return Error(loc,
                   "maximal shardings should have exactly one device assigned");
    }
    sharding->set_type(OpSharding::Type::OpSharding_Type_MAXIMAL);
    sharding->add_tile_assignment_devices(devices[0]);
  } else {
    if (devices.size() <= 1) {
      return Error(
          loc, "non-maximal shardings must have more than one device assigned");
    }
    if (tile_assignment_dimensions.empty()) {
      return Error(
          loc,
          "non-maximal shardings must have a tile assignment list including "
          "dimensions");
    }
    sharding->set_type(OpSharding::Type::OpSharding_Type_OTHER);
    for (tensorflow::int64 dim : tile_assignment_dimensions) {
      sharding->add_tile_assignment_dimensions(dim);
    }
    for (tensorflow::int64 device : devices) {
      sharding->add_tile_assignment_devices(device);
    }
  }

  lexer_.Lex();
  return true;
}

// domain ::= '{' 'kind=' domain_kind ',' 'entry=' entry_sharding ','
//            'exit=' exit_sharding '}'
bool HloParser::ParseDomain(DomainData* domain) {
  std::unordered_map<string, AttrConfig> attrs;
  optional<string> kind;
  optional<OpSharding> entry_sharding;
  optional<OpSharding> exit_sharding;
  attrs["kind"] = {/*required=*/true, AttrTy::kString, &kind};
  attrs["entry"] = {/*required=*/true, AttrTy::kSharding, &entry_sharding};
  attrs["exit"] = {/*required=*/true, AttrTy::kSharding, &exit_sharding};
  if (!ParseSubAttributes(attrs)) {
    return false;
  }
  if (*kind == ShardingMetadata::KindName()) {
    auto entry_sharding_ptr = absl::make_unique<HloSharding>(
        HloSharding::FromProto(*entry_sharding).ValueOrDie());
    auto exit_sharding_ptr = absl::make_unique<HloSharding>(
        HloSharding::FromProto(*exit_sharding).ValueOrDie());
    domain->entry_metadata =
        absl::make_unique<ShardingMetadata>(std::move(entry_sharding_ptr));
    domain->exit_metadata =
        absl::make_unique<ShardingMetadata>(std::move(exit_sharding_ptr));
  } else {
    return TokenError(StrCat("unsupported domain kind: ", *kind));
  }
  return true;
}

// '{' name+ '}'
bool HloParser::ParseInstructionNames(
    std::vector<HloInstruction*>* instructions) {
  if (!ParseToken(TokKind::kLbrace,
                  "expects '{' at the beginning of instruction name list")) {
    return false;
  }
  LocTy loc = lexer_.GetLoc();
  do {
    string name;
    if (!ParseName(&name)) {
      return Error(loc, "expects a instruction name");
    }
    std::pair<HloInstruction*, LocTy>* instr = FindInstruction(name);
    if (!instr) {
      return TokenError(StrFormat("instruction '%s' is not defined", name));
    }
    instructions->push_back(instr->first);
  } while (EatIfPresent(TokKind::kComma));

  return ParseToken(TokKind::kRbrace,
                    "expects '}' at the end of instruction name list");
}

bool HloParser::SetValueInLiteral(tensorflow::int64 value,
                                  tensorflow::int64 linear_index,
                                  Literal* literal) {
  const Shape& shape = literal->shape();
  switch (shape.element_type()) {
    case S8:
      return SetValueInLiteralHelper<tensorflow::int8>(value, linear_index,
                                                       literal);
    case S16:
      return SetValueInLiteralHelper<tensorflow::int16>(value, linear_index,
                                                        literal);
    case S32:
      return SetValueInLiteralHelper<tensorflow::int32>(value, linear_index,
                                                        literal);
    case S64:
      return SetValueInLiteralHelper<tensorflow::int64>(value, linear_index,
                                                        literal);
    case U8:
      return SetValueInLiteralHelper<tensorflow::uint8>(value, linear_index,
                                                        literal);
    case U16:
      return SetValueInLiteralHelper<tensorflow::uint16>(value, linear_index,
                                                         literal);
    case U32:
      return SetValueInLiteralHelper<tensorflow::uint32>(value, linear_index,
                                                         literal);
    case U64:
      return SetValueInLiteralHelper<tensorflow::uint64>(value, linear_index,
                                                         literal);
    case PRED:
      // Bool type literals with rank >= 1 are printed in 0s and 1s.
      return SetValueInLiteralHelper<bool>(static_cast<bool>(value),
                                           linear_index, literal);
    default:
      LOG(FATAL) << "unknown integral primitive type "
                 << PrimitiveType_Name(shape.element_type());
  }
}

bool HloParser::SetValueInLiteral(double value, tensorflow::int64 linear_index,
                                  Literal* literal) {
  const Shape& shape = literal->shape();
  switch (shape.element_type()) {
    case F16:
      return SetValueInLiteralHelper<Eigen::half>(value, linear_index, literal);
    case BF16:
      return SetValueInLiteralHelper<tensorflow::bfloat16>(value, linear_index,
                                                           literal);
    case F32:
      return SetValueInLiteralHelper<float>(value, linear_index, literal);
    case F64:
      return SetValueInLiteralHelper<double>(value, linear_index, literal);
    default:
      LOG(FATAL) << "unknown floating point primitive type "
                 << PrimitiveType_Name(shape.element_type());
  }
}

bool HloParser::SetValueInLiteral(bool value, tensorflow::int64 linear_index,
                                  Literal* literal) {
  const Shape& shape = literal->shape();
  switch (shape.element_type()) {
    case PRED:
      return SetValueInLiteralHelper<bool>(value, linear_index, literal);
    default:
      LOG(FATAL) << PrimitiveType_Name(shape.element_type())
                 << " is not PRED type";
  }
}

template <typename LiteralNativeT, typename ParsedElemT>
bool HloParser::SetValueInLiteralHelper(ParsedElemT value,
                                        tensorflow::int64 linear_index,
                                        Literal* literal) {
  // Check that linear_index is in range.
  if (linear_index >= ShapeUtil::ElementsIn(literal->shape())) {
    return TokenError(
        StrCat("trys to set value ", value, " to a literal in shape ",
               ShapeUtil::HumanString(literal->shape()), " at linear index ",
               linear_index, ", but the index is out of range"));
  }

  if (std::isnan(value) ||
      (std::numeric_limits<ParsedElemT>::has_infinity &&
       (std::numeric_limits<ParsedElemT>::infinity() == value ||
        -std::numeric_limits<ParsedElemT>::infinity() == value))) {
    // Skip range checking for non-finite value.
  } else if (literal->shape().element_type() == F16 ||
             literal->shape().element_type() == BF16) {
    if (value > kF16max || value < -kF16max) {
      return TokenError(StrCat(
          "value ", value, " is out of range for literal's primitive type ",
          PrimitiveType_Name(literal->shape().element_type())));
    }
  } else if (std::is_unsigned<LiteralNativeT>::value) {
    CHECK((std::is_same<ParsedElemT, tensorflow::int64>::value ||
           std::is_same<ParsedElemT, bool>::value))
        << "Unimplemented checking for ParsedElemT";

    ParsedElemT upper_bound;
    if (sizeof(LiteralNativeT) >= sizeof(ParsedElemT)) {
      upper_bound = std::numeric_limits<ParsedElemT>::max();
    } else {
      upper_bound =
          static_cast<ParsedElemT>(std::numeric_limits<LiteralNativeT>::max());
    }
    if (value > upper_bound || value < 0) {
      // Value is out of range for LiteralNativeT.
      return TokenError(StrCat(
          "value ", value, " is out of range for literal's primitive type ",
          PrimitiveType_Name(literal->shape().element_type())));
    }
  } else if (value > static_cast<ParsedElemT>(
                         std::numeric_limits<LiteralNativeT>::max()) ||
             value < static_cast<ParsedElemT>(
                         std::numeric_limits<LiteralNativeT>::lowest())) {
    // Value is out of range for LiteralNativeT.
    return TokenError(StrCat(
        "value ", value, " is out of range for literal's primitive type ",
        PrimitiveType_Name(literal->shape().element_type())));
  }

  literal->data<LiteralNativeT>().at(linear_index) =
      static_cast<LiteralNativeT>(value);
  return true;
}

bool HloParser::EatShapeAndCheckCompatible(const Shape& shape) {
  Shape new_shape;
  if (!ParseShape(&new_shape)) {
    return TokenError(StrCat("expects shape ", ShapeUtil::HumanString(shape)));
  }
  if (!ShapeUtil::Compatible(shape, new_shape)) {
    return TokenError(StrCat(
        "expects shape ", ShapeUtil::HumanString(shape),
        ", but sees a different shape: ", ShapeUtil::HumanString(new_shape)));
  }
  return true;
}

// literal
//  ::= tuple
//  ::= non_tuple
bool HloParser::ParseLiteral(Literal* literal, const Shape& shape) {
  return ShapeUtil::IsTuple(shape) ? ParseTupleLiteral(literal, shape)
                                   : ParseNonTupleLiteral(literal, shape);
}

// tuple
//  ::= shape '(' literal_list ')'
// literal_list
//  ::= /*empty*/
//  ::= literal (',' literal)*
bool HloParser::ParseTupleLiteral(Literal* literal, const Shape& shape) {
  if (!EatShapeAndCheckCompatible(shape)) {
    return TokenError(StrCat("expects tuple constant in shape ",
                             ShapeUtil::HumanString(shape)));
  }
  if (!ParseToken(TokKind::kLparen, "expects '(' in front of tuple elements")) {
    return false;
  }
  std::vector<Literal> elements(ShapeUtil::TupleElementCount(shape));

  if (lexer_.GetKind() == TokKind::kRparen) {
    // empty
  } else {
    // literal, (',' literal)*
    for (int i = 0; i < elements.size(); i++) {
      if (i > 0) {
        ParseToken(TokKind::kComma, "exepcts ',' to separate tuple elements");
      }
      if (!ParseLiteral(&elements[i],
                        ShapeUtil::GetTupleElementShape(shape, i))) {
        return TokenError(StrCat("expects the ", i, "th element"));
      }
    }
  }
  *literal = LiteralUtil::MakeTupleOwned(std::move(elements));
  return ParseToken(TokKind::kRparen,
                    StrCat("expects ')' at the end of the tuple with ",
                           ShapeUtil::TupleElementCount(shape), "elements"));
}

// non_tuple
//   ::= rank01
//   ::= rank2345
// rank2345 ::= shape sparse_or_nested_array
bool HloParser::ParseNonTupleLiteral(Literal* literal, const Shape& shape) {
  if (LayoutUtil::IsSparseArray(shape)) {
    return ParseSparseLiteral(literal, shape);
  }

  CHECK(LayoutUtil::IsDenseArray(shape));
  return ParseDenseLiteral(literal, shape);
}

bool HloParser::ParseDenseLiteral(Literal* literal, const Shape& shape) {
  const tensorflow::int64 rank = ShapeUtil::Rank(shape);
  if (rank > 1 && !EatShapeAndCheckCompatible(shape)) {
    return false;
  }

  // Create a literal with the given shape in default layout.
  *literal = LiteralUtil::CreateFromDimensions(
      shape.element_type(), AsInt64Slice(shape.dimensions()));
  tensorflow::int64 nest_level = 0;
  tensorflow::int64 linear_index = 0;
  // elems_seen_per_dim[i] is how many elements or sub-arrays we have seen for
  // the dimension i. For example, to parse f32[2,3] {{1, 2, 3}, {4, 5, 6}},
  // when we are parsing the 2nd '{' (right before '1'), we are seeing a
  // sub-array of the dimension 0, so elems_seen_per_dim[0]++. When we are at
  // the first '}' (right after '3'), it means the sub-array ends, and the
  // sub-array is supposed to contain exactly 3 elements, so check if
  // elems_seen_per_dim[1] is 3.
  std::vector<tensorflow::int64> elems_seen_per_dim(rank);
  auto get_index_str = [&elems_seen_per_dim](int dim) -> string {
    std::vector<tensorflow::int64> elems_seen_until_dim(
        elems_seen_per_dim.begin(), elems_seen_per_dim.begin() + dim);
    return StrCat("[",
                  StrJoin(elems_seen_until_dim, ",",
                          [](string* out, const tensorflow::int64& num_elems) {
                            StrAppend(out, num_elems - 1);
                          }),
                  "]");
  };
  do {
    switch (lexer_.GetKind()) {
      default:
        return TokenError("unexpected token type in a literal");
      case TokKind::kLbrace: {
        nest_level++;
        if (nest_level > rank) {
          return TokenError(absl::StrFormat(
              "expects nested array in rank %d, but sees larger", rank));
        }
        if (nest_level > 1) {
          elems_seen_per_dim[nest_level - 2]++;
          if (elems_seen_per_dim[nest_level - 2] >
              shape.dimensions(nest_level - 2)) {
            return TokenError(absl::StrFormat(
                "expects %d elements in the %sth element, but sees more",
                shape.dimensions(nest_level - 2),
                get_index_str(nest_level - 2)));
          }
        }
        lexer_.Lex();
        break;
      }
      case TokKind::kRbrace: {
        nest_level--;
        if (elems_seen_per_dim[nest_level] != shape.dimensions(nest_level)) {
          return TokenError(absl::StrFormat(
              "expects %d elements in the %sth element, but sees %d",
              shape.dimensions(nest_level), get_index_str(nest_level),
              elems_seen_per_dim[nest_level]));
        }
        elems_seen_per_dim[nest_level] = 0;
        lexer_.Lex();
        break;
      }
      case TokKind::kComma:
        // Skip.
        lexer_.Lex();
        break;
      case TokKind::kw_true:
      case TokKind::kw_false:
      case TokKind::kInt:
      case TokKind::kDecimal:
      case TokKind::kw_nan:
      case TokKind::kw_inf:
      case TokKind::kNegInf: {
        if (rank > 0) {
          if (nest_level != rank) {
            return TokenError(
                absl::StrFormat("expects nested array in rank %d, but sees %d",
                                rank, nest_level));
          }
          elems_seen_per_dim[rank - 1]++;
          if (elems_seen_per_dim[rank - 1] > shape.dimensions(rank - 1)) {
            return TokenError(absl::StrFormat(
                "expects %d elements on the minor-most dimension, but "
                "sees more",
                shape.dimensions(rank - 1)));
          }
        }
        if (lexer_.GetKind() == TokKind::kw_true ||
            lexer_.GetKind() == TokKind::kw_false) {
          if (!SetValueInLiteral(lexer_.GetKind() == TokKind::kw_true,
                                 linear_index++, literal)) {
            return false;
          }
          lexer_.Lex();
        } else if (primitive_util::IsIntegralType(shape.element_type()) ||
                   shape.element_type() == PRED) {
          LocTy loc = lexer_.GetLoc();
          tensorflow::int64 value;
          if (!ParseInt64(&value)) {
            return Error(loc, StrCat("expects integer for primitive type: ",
                                     PrimitiveType_Name(shape.element_type())));
          }
          if (!SetValueInLiteral(value, linear_index++, literal)) {
            return false;
          }
        } else if (primitive_util::IsFloatingPointType(shape.element_type())) {
          LocTy loc = lexer_.GetLoc();
          double value;
          if (!ParseDouble(&value)) {
            return Error(
                loc, StrCat("expect floating point value for primitive type: ",
                            PrimitiveType_Name(shape.element_type())));
          }
          if (!SetValueInLiteral(value, linear_index++, literal)) {
            return false;
          }
        } else {
          return TokenError(StrCat("unsupported primitive type ",
                                   PrimitiveType_Name(shape.element_type())));
        }
        break;
      }
    }  // end of switch
  } while (nest_level > 0);

  *literal = literal->Relayout(shape.layout());
  return true;
}

bool HloParser::ParseSparseLiteral(Literal* literal, const Shape& shape) {
  if (!EatShapeAndCheckCompatible(shape)) {
    return false;
  }

  switch (shape.element_type()) {
    case PRED:
      return ParseSparseLiteralHelper<tensorflow::uint8>(literal, shape);
    case S8:
      return ParseSparseLiteralHelper<tensorflow::int8>(literal, shape);
    case S16:
      return ParseSparseLiteralHelper<tensorflow::int16>(literal, shape);
    case S32:
      return ParseSparseLiteralHelper<tensorflow::int32>(literal, shape);
    case S64:
      return ParseSparseLiteralHelper<tensorflow::int64>(literal, shape);
    case U8:
      return ParseSparseLiteralHelper<tensorflow::uint8>(literal, shape);
    case U16:
      return ParseSparseLiteralHelper<tensorflow::uint16>(literal, shape);
    case U32:
      return ParseSparseLiteralHelper<tensorflow::uint32>(literal, shape);
    case U64:
      return ParseSparseLiteralHelper<tensorflow::uint64>(literal, shape);
    case F16:
      return ParseSparseLiteralHelper<Eigen::half>(literal, shape);
    case F32:
      return ParseSparseLiteralHelper<float>(literal, shape);
    case BF16:
      return ParseSparseLiteralHelper<tensorflow::bfloat16>(literal, shape);
    case F64:
      return ParseSparseLiteralHelper<double>(literal, shape);
    default:
      return Error(lexer_.GetLoc(),
                   StrCat("invalid primitive type for sparse literal: ",
                          PrimitiveType_Name(shape.element_type())));
  }
}

template <typename LiteralNativeT>
bool HloParser::ParseSparseLiteralHelper(Literal* literal, const Shape& shape) {
  std::vector<tensorflow::int64> index;

  tensorflow::int64 rank = ShapeUtil::Rank(shape);

  *literal = Literal(shape);

  if (!ParseToken(TokKind::kLbrace,
                  "expects '{' at the beginning of a sparse literal")) {
    return false;
  }

  for (;;) {
    if (lexer_.GetKind() == TokKind::kRbrace) {
      lexer_.Lex();
      break;
    }

    LocTy index_loc = lexer_.GetLoc();
    index.clear();
    if (lexer_.GetKind() == TokKind::kInt) {
      tensorflow::int64 single_index = lexer_.GetInt64Val();
      lexer_.Lex();
      if (rank != 1) {
        return Error(
            index_loc,
            StrCat("invalid single-dimensional index for shape with rank ",
                   rank, ": ", single_index));
      }
      index.push_back(single_index);
    } else {
      if (!ParseInt64List(TokKind::kLsquare, TokKind::kRsquare, TokKind::kComma,
                          &index)) {
        return false;
      }
      if (index.size() != rank) {
        return Error(
            index_loc,
            StrCat("invalid multi-dimension index for shape with rank ", rank,
                   ": [", StrJoin(index, ", "), "]"));
      }
    }
    if (!ParseToken(TokKind::kColon,
                    "expects ':' after after the sparse array index and before "
                    "the sparse array value")) {
      return false;
    }
    LocTy value_loc = lexer_.GetLoc();
    LiteralNativeT value;
    if (lexer_.GetKind() == TokKind::kw_true ||
        lexer_.GetKind() == TokKind::kw_false) {
      value = static_cast<LiteralNativeT>(lexer_.GetKind() == TokKind::kw_true);
      lexer_.Lex();
    } else if (primitive_util::IsIntegralType(shape.element_type())) {
      tensorflow::int64 value_s64;
      if (!ParseInt64(&value_s64)) {
        return Error(value_loc,
                     StrCat("expects integer for primitive type: ",
                            PrimitiveType_Name(shape.element_type())));
      }
      value = static_cast<LiteralNativeT>(value_s64);
    } else if (primitive_util::IsFloatingPointType(shape.element_type())) {
      double value_f64;
      if (!ParseDouble(&value_f64)) {
        return Error(value_loc,
                     StrCat("expects floating point value for primitive type: ",
                            PrimitiveType_Name(shape.element_type())));
      }
      value = static_cast<LiteralNativeT>(value_f64);
    } else {
      LOG(FATAL) << "Unexpected element type: "
                 << PrimitiveType_Name(shape.element_type());
    }
    if (lexer_.GetKind() != TokKind::kRbrace &&
        !ParseToken(TokKind::kComma,
                    "expects ',' separator between sparse array elements")) {
      return false;
    }

    if (literal->sparse_element_count() + 1 ==
        LayoutUtil::MaxSparseElements(shape.layout())) {
      return Error(
          lexer_.GetLoc(),
          StrCat("number of sparse elements exceeds maximum for layout: ",
                 ShapeUtil::HumanStringWithLayout(shape)));
    }

    literal->AppendSparseElement(index, value);
  }

  literal->SortSparseElements();
  return true;
}

// operands ::= '(' operands1 ')'
// operands1
//   ::= /*empty*/
//   ::= operand (, operand)*
// operand ::= (shape)? name
bool HloParser::ParseOperands(std::vector<HloInstruction*>* operands) {
  CHECK(operands != nullptr);
  if (!ParseToken(TokKind::kLparen,
                  "expects '(' at the beginning of operands")) {
    return false;
  }
  if (lexer_.GetKind() == TokKind::kRparen) {
    // empty
  } else {
    do {
      LocTy loc = lexer_.GetLoc();
      string name;
      optional<Shape> shape;
      if (CanBeShape()) {
        shape.emplace();
        if (!ParseShape(&shape.value())) {
          return false;
        }
      }
      if (!ParseName(&name)) {
        // When parsing a single instruction (as opposed to a whole module), an
        // HLO may have one or more operands with a shape but no name:
        //
        //  foo = add(f32[10], f32[10])
        //
        // create_missing_instruction_ is always non-null when parsing a single
        // instruction, and is responsible for creating kParameter instructions
        // for these operands.
        if (shape.has_value() && create_missing_instruction_ != nullptr &&
            scoped_name_tables_.size() == 1) {
          name = "";
        } else {
          return false;
        }
      }
      std::pair<HloInstruction*, LocTy>* instruction =
          FindInstruction(name, shape);
      if (instruction == nullptr) {
        return Error(loc, StrCat("instruction does not exist: ", name));
      }
      operands->push_back(instruction->first);
    } while (EatIfPresent(TokKind::kComma));
  }
  return ParseToken(TokKind::kRparen, "expects ')' at the end of operands");
}

bool HloParser::ParseOperands(std::vector<HloInstruction*>* operands,
                              const int expected_size) {
  CHECK(operands != nullptr);
  LocTy loc = lexer_.GetLoc();
  if (!ParseOperands(operands)) {
    return false;
  }
  if (expected_size != operands->size()) {
    return Error(loc, StrCat("expects ", expected_size, " operands, but has ",
                             operands->size(), " operands"));
  }
  return true;
}

// sub_attributes ::= '{' (','? attribute)* '}'
bool HloParser::ParseSubAttributes(
    const std::unordered_map<string, AttrConfig>& attrs) {
  LocTy loc = lexer_.GetLoc();
  if (!ParseToken(TokKind::kLbrace, "expects '{' to start sub attributes")) {
    return false;
  }
  std::unordered_set<string> seen_attrs;
  if (lexer_.GetKind() == TokKind::kRbrace) {
    // empty
  } else {
    do {
      EatIfPresent(TokKind::kComma);
      if (!ParseAttributeHelper(attrs, &seen_attrs)) {
        return false;
      }
    } while (lexer_.GetKind() != TokKind::kRbrace);
  }
  // Check that all required attrs were seen.
  for (const auto& attr_it : attrs) {
    if (attr_it.second.required &&
        seen_attrs.find(attr_it.first) == seen_attrs.end()) {
      return Error(loc, StrFormat("sub-attribute %s is expected but not seen",
                                  attr_it.first));
    }
  }
  return ParseToken(TokKind::kRbrace, "expects '}' to end sub attributes");
}

// attributes ::= (',' attribute)*
bool HloParser::ParseAttributes(
    const std::unordered_map<string, AttrConfig>& attrs) {
  LocTy loc = lexer_.GetLoc();
  std::unordered_set<string> seen_attrs;
  while (EatIfPresent(TokKind::kComma)) {
    if (!ParseAttributeHelper(attrs, &seen_attrs)) {
      return false;
    }
  }
  // Check that all required attrs were seen.
  for (const auto& attr_it : attrs) {
    if (attr_it.second.required &&
        seen_attrs.find(attr_it.first) == seen_attrs.end()) {
      return Error(loc, StrFormat("attribute %s is expected but not seen",
                                  attr_it.first));
    }
  }
  return true;
}

bool HloParser::ParseAttributeHelper(
    const std::unordered_map<string, AttrConfig>& attrs,
    std::unordered_set<string>* seen_attrs) {
  LocTy loc = lexer_.GetLoc();
  string name;
  if (!ParseAttributeName(&name)) {
    return Error(loc, "error parsing attributes");
  }
  VLOG(1) << "Parsing attribute " << name;
  if (!seen_attrs->insert(name).second) {
    return Error(loc, StrFormat("attribute %s already exists", name));
  }
  auto attr_it = attrs.find(name);
  if (attr_it == attrs.end()) {
    string allowed_attrs;
    if (attrs.empty()) {
      allowed_attrs = "No attributes are allowed here.";
    } else {
      allowed_attrs = StrCat(
          "Allowed attributes: ",
          StrJoin(attrs, ", ",
                  [&](string* out, const std::pair<string, AttrConfig>& kv) {
                    StrAppend(out, kv.first);
                  }));
    }
    return Error(loc, StrFormat("unexpected attribute \"%s\".  %s", name,
                                allowed_attrs));
  }
  AttrTy attr_type = attr_it->second.attr_type;
  void* attr_out_ptr = attr_it->second.result;
  bool success = [&] {
    LocTy attr_loc = lexer_.GetLoc();
    switch (attr_type) {
      case AttrTy::kBool: {
        bool result;
        if (!ParseBool(&result)) {
          return false;
        }
        static_cast<optional<bool>*>(attr_out_ptr)->emplace(result);
        return true;
      }
      case AttrTy::kInt64: {
        tensorflow::int64 result;
        if (!ParseInt64(&result)) {
          return false;
        }
        static_cast<optional<tensorflow::int64>*>(attr_out_ptr)
            ->emplace(result);
        return true;
      }
      case AttrTy::kInt32: {
        tensorflow::int64 result;
        if (!ParseInt64(&result)) {
          return false;
        }
        if (result != static_cast<tensorflow::int32>(result)) {
          return Error(attr_loc, "value out of range for int32");
        }
        static_cast<optional<tensorflow::int32>*>(attr_out_ptr)
            ->emplace(static_cast<tensorflow::int32>(result));
        return true;
      }
      case AttrTy::kFloat: {
        double result;
        if (!ParseDouble(&result)) {
          return false;
        }
        if (result > std::numeric_limits<float>::max() ||
            result < std::numeric_limits<float>::lowest()) {
          return Error(attr_loc, "value out of range for float");
        }
        static_cast<optional<float>*>(attr_out_ptr)
            ->emplace(static_cast<float>(result));
        return true;
      }
      case AttrTy::kHloComputation: {
        HloComputation* result = nullptr;
        if (lexer_.GetKind() == TokKind::kLbrace) {
          // This means it is a nested computation.
          if (!ParseInstructionList(&result, /*computation_name=*/"_")) {
            return false;
          }
        } else {
          // This means it is a computation name.
          if (!ParseComputationName(&result)) {
            return false;
          }
        }
        static_cast<optional<HloComputation*>*>(attr_out_ptr)->emplace(result);
        return true;
      }
      case AttrTy::kFftType: {
        FftType result;
        if (!ParseFftType(&result)) {
          return false;
        }
        static_cast<optional<FftType>*>(attr_out_ptr)->emplace(result);
        return true;
      }
      case AttrTy::kWindow: {
        Window result;
        if (!ParseWindow(&result, /*expect_outer_curlies=*/true)) {
          return false;
        }
        static_cast<optional<Window>*>(attr_out_ptr)->emplace(result);
        return true;
      }
      case AttrTy::kConvolutionDimensionNumbers: {
        ConvolutionDimensionNumbers result;
        if (!ParseConvolutionDimensionNumbers(&result)) {
          return false;
        }
        static_cast<optional<ConvolutionDimensionNumbers>*>(attr_out_ptr)
            ->emplace(result);
        return true;
      }
      case AttrTy::kSharding: {
        OpSharding sharding;
        if (!ParseSharding(&sharding)) {
          return false;
        }
        static_cast<optional<OpSharding>*>(attr_out_ptr)->emplace(sharding);
        return true;
      }
      case AttrTy::kInstructionList: {
        std::vector<HloInstruction*> result;
        if (!ParseInstructionNames(&result)) {
          return false;
        }
        static_cast<optional<std::vector<HloInstruction*>>*>(attr_out_ptr)
            ->emplace(result);
        return true;
      }
      case AttrTy::kFusionKind: {
        HloInstruction::FusionKind result;
        if (!ParseFusionKind(&result)) {
          return false;
        }
        static_cast<optional<HloInstruction::FusionKind>*>(attr_out_ptr)
            ->emplace(result);
        return true;
      }
      case AttrTy::kBracedInt64List: {
        std::vector<tensorflow::int64> result;
        if (!ParseInt64List(TokKind::kLbrace, TokKind::kRbrace, TokKind::kComma,
                            &result)) {
          return false;
        }
        static_cast<optional<std::vector<tensorflow::int64>>*>(attr_out_ptr)
            ->emplace(result);
        return true;
      }
      case AttrTy::kBracedInt64ListList: {
        std::vector<std::vector<tensorflow::int64>> result;
        auto parse_and_add_item = [&]() {
          std::vector<tensorflow::int64> item;
          if (!ParseInt64List(TokKind::kLbrace, TokKind::kRbrace,
                              TokKind::kComma, &item)) {
            return false;
          }
          result.push_back(item);
          return true;
        };
        if (!ParseList(TokKind::kLbrace, TokKind::kRbrace, TokKind::kComma,
                       parse_and_add_item)) {
          return false;
        }
        static_cast<optional<std::vector<std::vector<tensorflow::int64>>>*>(
            attr_out_ptr)
            ->emplace(result);
        return true;
      }
      case AttrTy::kSliceRanges: {
        SliceRanges result;
        if (!ParseSliceRanges(&result)) {
          return false;
        }
        static_cast<optional<SliceRanges>*>(attr_out_ptr)->emplace(result);
        return true;
      }
      case AttrTy::kPaddingConfig: {
        PaddingConfig result;
        if (!ParsePaddingConfig(&result)) {
          return false;
        }
        static_cast<optional<PaddingConfig>*>(attr_out_ptr)->emplace(result);
        return true;
      }
      case AttrTy::kString: {
        string result;
        if (!ParseString(&result)) {
          return false;
        }
        static_cast<optional<string>*>(attr_out_ptr)->emplace(result);
        return true;
      }
      case AttrTy::kMetadata: {
        OpMetadata result;
        if (!ParseMetadata(&result)) {
          return false;
        }
        static_cast<optional<OpMetadata>*>(attr_out_ptr)->emplace(result);
        return true;
      }
      case AttrTy::kDistribution: {
        RandomDistribution result;
        if (!ParseRandomDistribution(&result)) {
          return false;
        }
        static_cast<optional<RandomDistribution>*>(attr_out_ptr)
            ->emplace(result);
        return true;
      }
      case AttrTy::kDomain: {
        return ParseDomain(static_cast<DomainData*>(attr_out_ptr));
      }
      case AttrTy::kPrecisionList: {
        std::vector<PrecisionConfig::Precision> result;
        if (!ParsePrecisionList(&result)) {
          return false;
        }
        static_cast<optional<std::vector<PrecisionConfig::Precision>>*>(
            attr_out_ptr)
            ->emplace(result);
        return true;
      }
      case AttrTy::kShapeList: {
        std::vector<Shape> result;
        if (!ParseShapeList(&result)) {
          return false;
        }
        static_cast<optional<std::vector<Shape>>*>(attr_out_ptr)
            ->emplace(result);
        return true;
      }
    }
  }();
  if (!success) {
    return Error(loc, StrFormat("error parsing attribute %s", name));
  }
  return true;
}

bool HloParser::ParseComputationName(HloComputation** value) {
  string name;
  LocTy loc = lexer_.GetLoc();
  if (!ParseName(&name)) {
    return Error(loc, "expects computation name");
  }
  std::pair<HloComputation*, LocTy>* computation =
      tensorflow::gtl::FindOrNull(computation_pool_, name);
  if (computation == nullptr) {
    return Error(loc, StrCat("computation does not exist: ", name));
  }
  *value = computation->first;
  return true;
}

// ::= '{' size stride? pad? lhs_dilate? rhs_dilate? '}'
// The subattributes can appear in any order. 'size=' is required, others are
// optional.
bool HloParser::ParseWindow(Window* window, bool expect_outer_curlies) {
  LocTy loc = lexer_.GetLoc();
  if (expect_outer_curlies &&
      !ParseToken(TokKind::kLbrace, "expected '{' to start window attribute")) {
    return false;
  }

  std::vector<int64> size;
  std::vector<int64> stride;
  std::vector<std::vector<int64>> pad;
  std::vector<int64> lhs_dilate;
  std::vector<int64> rhs_dilate;
  std::vector<int64> rhs_reversal;
  const auto end_token =
      expect_outer_curlies ? TokKind::kRbrace : TokKind::kEof;
  while (lexer_.GetKind() != end_token) {
    LocTy attr_loc = lexer_.GetLoc();
    string field_name;
    if (!ParseAttributeName(&field_name)) {
      return Error(attr_loc, "expects sub-attributes in window");
    }
    bool ok = [&] {
      if (field_name == "size") {
        return ParseDxD("size", &size);
      }
      if (field_name == "stride") {
        return ParseDxD("stride", &stride);
      }
      if (field_name == "lhs_dilate") {
        return ParseDxD("lhs_dilate", &lhs_dilate);
      }
      if (field_name == "rhs_dilate") {
        return ParseDxD("rls_dilate", &rhs_dilate);
      }
      if (field_name == "pad") {
        return ParseWindowPad(&pad);
      }
      if (field_name == "rhs_reversal") {
        return ParseDxD("rhs_reversal", &rhs_reversal);
      }
      return Error(attr_loc, StrCat("unexpected attribute name: ", field_name));
    }();
    if (!ok) {
      return false;
    }
  }

  if (size.empty()) {
    return Error(loc,
                 "sub-attribute 'size=' is required in the window attribute");
  }
  if (!stride.empty() && stride.size() != size.size()) {
    return Error(loc, "expects 'stride=' has the same size as 'size='");
  }
  if (!lhs_dilate.empty() && lhs_dilate.size() != size.size()) {
    return Error(loc, "expects 'lhs_dilate=' has the same size as 'size='");
  }
  if (!rhs_dilate.empty() && rhs_dilate.size() != size.size()) {
    return Error(loc, "expects 'rhs_dilate=' has the same size as 'size='");
  }
  if (!pad.empty() && pad.size() != size.size()) {
    return Error(loc, "expects 'pad=' has the same size as 'size='");
  }

  for (int i = 0; i < size.size(); i++) {
    window->add_dimensions()->set_size(size[i]);
    if (!pad.empty()) {
      window->mutable_dimensions(i)->set_padding_low(pad[i][0]);
      window->mutable_dimensions(i)->set_padding_high(pad[i][1]);
    }
    // If some field is not present, it has the default value.
    window->mutable_dimensions(i)->set_stride(stride.empty() ? 1 : stride[i]);
    window->mutable_dimensions(i)->set_base_dilation(
        lhs_dilate.empty() ? 1 : lhs_dilate[i]);
    window->mutable_dimensions(i)->set_window_dilation(
        rhs_dilate.empty() ? 1 : rhs_dilate[i]);
    window->mutable_dimensions(i)->set_window_reversal(
        rhs_reversal.empty() ? false : (rhs_reversal[i] == 1));
  }
  return !expect_outer_curlies ||
         ParseToken(TokKind::kRbrace, "expected '}' to end window attribute");
}

// This is the inverse of HloInstruction::ConvolutionDimensionNumbersToString.
// The string looks like "dim_labels=0bf_0io->0bf".
bool HloParser::ParseConvolutionDimensionNumbers(
    ConvolutionDimensionNumbers* dnums) {
  if (lexer_.GetKind() != TokKind::kDimLabels) {
    return TokenError("expects dim labels pattern, e.g., 'bf0_0io->0bf'");
  }
  string str = lexer_.GetStrVal();

  // The str is expected to have 3 items, lhs, rhs, out, and it must look like
  // lhs_rhs->out, that is, the first separator is "_" and the second is "->".
  std::vector<string> split1 = absl::StrSplit(str, '_');
  if (split1.size() != 2) {
    LOG(FATAL) << "expects 3 items: lhs, rhs, and output dims, but sees "
               << str;
  }
  std::vector<string> split2 = absl::StrSplit(split1[1], "->");
  if (split2.size() != 2) {
    LOG(FATAL) << "expects 3 items: lhs, rhs, and output dims, but sees "
               << str;
  }
  absl::string_view lhs = split1[0];
  absl::string_view rhs = split2[0];
  absl::string_view out = split2[1];

  const tensorflow::int64 rank = lhs.length();
  if (rank != rhs.length() || rank != out.length()) {
    return TokenError(
        "convolution lhs, rhs, and output must have the same rank");
  }
  if (rank < 2) {
    return TokenError("convolution rank must >=2");
  }

  auto is_unique = [](string str) -> bool {
    std::sort(str.begin(), str.end());
    return std::unique(str.begin(), str.end()) == str.end();
  };

  // lhs
  {
    if (!is_unique(string(lhs))) {
      return TokenError(
          StrCat("expects unique lhs dimension numbers, but sees ", lhs));
    }
    for (int i = 0; i < rank - 2; i++) {
      dnums->add_input_spatial_dimensions(-1);
    }
    for (int i = 0; i < rank; i++) {
      char c = lhs[i];
      if (c == 'b') {
        dnums->set_input_batch_dimension(i);
      } else if (c == 'f') {
        dnums->set_input_feature_dimension(i);
      } else if (c < '0' + rank && c >= '0') {
        dnums->set_input_spatial_dimensions(c - '0', i);
      } else {
        return TokenError(
            StrFormat("expects [0-%dbf] in lhs dimension numbers", rank - 1));
      }
    }
  }
  // rhs
  {
    if (!is_unique(string(rhs))) {
      return TokenError(
          StrCat("expects unique rhs dimension numbers, but sees ", rhs));
    }
    for (int i = 0; i < rank - 2; i++) {
      dnums->add_kernel_spatial_dimensions(-1);
    }
    for (int i = 0; i < rank; i++) {
      char c = rhs[i];
      if (c == 'i') {
        dnums->set_kernel_input_feature_dimension(i);
      } else if (c == 'o') {
        dnums->set_kernel_output_feature_dimension(i);
      } else if (c < '0' + rank && c >= '0') {
        dnums->set_kernel_spatial_dimensions(c - '0', i);
      } else {
        return TokenError(
            StrFormat("expects [0-%dio] in rhs dimension numbers", rank - 1));
      }
    }
  }
  // output
  {
    if (!is_unique(string(out))) {
      return TokenError(
          StrCat("expects unique output dimension numbers, but sees ", out));
    }
    for (int i = 0; i < rank - 2; i++) {
      dnums->add_output_spatial_dimensions(-1);
    }
    for (int i = 0; i < rank; i++) {
      char c = out[i];
      if (c == 'b') {
        dnums->set_output_batch_dimension(i);
      } else if (c == 'f') {
        dnums->set_output_feature_dimension(i);
      } else if (c < '0' + rank && c >= '0') {
        dnums->set_output_spatial_dimensions(c - '0', i);
      } else {
        return TokenError(StrFormat(
            "expects [0-%dbf] in output dimension numbers", rank - 1));
      }
    }
  }

  lexer_.Lex();
  return true;
}

// ::= '{' ranges '}'
//   ::= /*empty*/
//   ::= range (',' range)*
// range ::= '[' start ':' limit (':' stride)? ']'
//
// The slice ranges are printed as:
//
//  {[dim0_start:dim0_limit:dim0stride], [dim1_start:dim1_limit], ...}
//
// This function extracts the starts, limits, and strides as 3 vectors to the
// result. If stride is not present, stride is 1. For example, if the slice
// ranges is printed as:
//
//  {[2:3:4], [5:6:7], [8:9]}
//
// The parsed result will be:
//
//  {/*starts=*/{2, 5, 8}, /*limits=*/{3, 6, 9}, /*strides=*/{4, 7, 1}}
//
bool HloParser::ParseSliceRanges(SliceRanges* result) {
  if (!ParseToken(TokKind::kLbrace, "expects '{' to start ranges")) {
    return false;
  }
  std::vector<std::vector<tensorflow::int64>> ranges;
  if (lexer_.GetKind() == TokKind::kRbrace) {
    // empty
    return ParseToken(TokKind::kRbrace, "expects '}' to end ranges");
  }
  do {
    LocTy loc = lexer_.GetLoc();
    ranges.emplace_back();
    if (!ParseInt64List(TokKind::kLsquare, TokKind::kRsquare, TokKind::kColon,
                        &ranges.back())) {
      return false;
    }
    const auto& range = ranges.back();
    if (range.size() != 2 && range.size() != 3) {
      return Error(loc,
                   StrFormat("expects [start:limit:step] or [start:limit], "
                             "but sees %d elements.",
                             range.size()));
    }
  } while (EatIfPresent(TokKind::kComma));

  for (const auto& range : ranges) {
    result->starts.push_back(range[0]);
    result->limits.push_back(range[1]);
    result->strides.push_back(range.size() == 3 ? range[2] : 1);
  }
  return ParseToken(TokKind::kRbrace, "expects '}' to end ranges");
}

// precisionlist ::= start precision_elements end
// precision_elements
//   ::= /*empty*/
//   ::= precision_val (delim precision_val)*
bool HloParser::ParsePrecisionList(
    std::vector<PrecisionConfig::Precision>* result) {
  auto parse_and_add_item = [&]() {
    PrecisionConfig::Precision item;
    if (!ParsePrecision(&item)) {
      return false;
    }
    result->push_back(item);
    return true;
  };
  return ParseList(TokKind::kLbrace, TokKind::kRbrace, TokKind::kComma,
                   parse_and_add_item);
}

// shapelist ::= '{' shapes '}'
// precision_elements
//   ::= /*empty*/
//   ::= shape (',' shape)*
bool HloParser::ParseShapeList(std::vector<Shape>* result) {
  auto parse_and_add_item = [&]() {
    Shape shape;
    if (!ParseShape(&shape)) {
      return false;
    }
    result->push_back(std::move(shape));
    return true;
  };
  return ParseList(TokKind::kLbrace, TokKind::kRbrace, TokKind::kComma,
                   parse_and_add_item);
}

// int64list ::= start int64_elements end
// int64_elements
//   ::= /*empty*/
//   ::= int64_val (delim int64_val)*
bool HloParser::ParseInt64List(const TokKind start, const TokKind end,
                               const TokKind delim,
                               std::vector<tensorflow::int64>* result) {
  auto parse_and_add_item = [&]() {
    tensorflow::int64 i;
    if (!ParseInt64(&i)) {
      return false;
    }
    result->push_back(i);
    return true;
  };
  return ParseList(start, end, delim, parse_and_add_item);
}

bool HloParser::ParseList(const TokKind start, const TokKind end,
                          const TokKind delim,
                          const std::function<bool()>& parse_and_add_item) {
  if (!ParseToken(start, StrCat("expects a list starting with ",
                                TokKindToString(start)))) {
    return false;
  }
  if (lexer_.GetKind() == end) {
    // empty
  } else {
    do {
      if (!parse_and_add_item()) {
        return false;
      }
    } while (EatIfPresent(delim));
  }
  return ParseToken(
      end, StrCat("expects a list to end with ", TokKindToString(end)));
}

// param_list_to_shape ::= param_list '->' shape
bool HloParser::ParseParamListToShape(Shape* shape, LocTy* shape_loc) {
  if (!ParseParamList() || !ParseToken(TokKind::kArrow, "expects '->'")) {
    return false;
  }
  *shape_loc = lexer_.GetLoc();
  return ParseShape(shape);
}

bool HloParser::CanBeParamListToShape() {
  return lexer_.GetKind() == TokKind::kLparen;
}

// param_list ::= '(' param_list1 ')'
// param_list1
//   ::= /*empty*/
//   ::= param (',' param)*
// param ::= name shape
bool HloParser::ParseParamList() {
  if (!ParseToken(TokKind::kLparen,
                  "expects '(' at the beginning of param list")) {
    return false;
  }

  if (lexer_.GetKind() == TokKind::kRparen) {
    // empty
  } else {
    do {
      Shape shape;
      string name;
      if (!ParseName(&name) || !ParseShape(&shape)) {
        return false;
      }
    } while (EatIfPresent(TokKind::kComma));
  }
  return ParseToken(TokKind::kRparen, "expects ')' at the end of param list");
}

// shape ::= shape_val_
// shape ::= '(' tuple_elements ')'
// tuple_elements
//   ::= /*empty*/
//   ::= shape (',' shape)*
bool HloParser::ParseShape(Shape* result) {
  if (EatIfPresent(TokKind::kLparen)) {  // Tuple
    std::vector<Shape> shapes;
    if (lexer_.GetKind() == TokKind::kRparen) {
      /*empty*/
    } else {
      // shape (',' shape)*
      do {
        shapes.emplace_back();
        if (!ParseShape(&shapes.back())) {
          return false;
        }
      } while (EatIfPresent(TokKind::kComma));
    }
    *result = ShapeUtil::MakeTupleShape(shapes);
    return ParseToken(TokKind::kRparen, "expects ')' at the end of tuple.");
  }

  if (lexer_.GetKind() != TokKind::kShape) {
    return TokenError(absl::StrCat("expected shape, saw ",
                                   TokKindToString(lexer_.GetKind())));
  }
  *result = lexer_.GetShapeVal();
  lexer_.Lex();
  return true;
}

bool HloParser::CanBeShape() {
  // A non-tuple shape starts with a kShape token; a tuple shape starts with
  // '('.
  return lexer_.GetKind() == TokKind::kShape ||
         lexer_.GetKind() == TokKind::kLparen;
}

bool HloParser::ParseName(string* result) {
  VLOG(1) << "ParseName";
  if (lexer_.GetKind() != TokKind::kIdent &&
      lexer_.GetKind() != TokKind::kName) {
    return TokenError("expects name");
  }
  *result = lexer_.GetStrVal();
  lexer_.Lex();
  return true;
}

bool HloParser::ParseAttributeName(string* result) {
  if (lexer_.GetKind() != TokKind::kAttributeName) {
    return TokenError("expects attribute name");
  }
  *result = lexer_.GetStrVal();
  lexer_.Lex();
  return true;
}

bool HloParser::ParseString(string* result) {
  VLOG(1) << "ParseString";
  if (lexer_.GetKind() != TokKind::kString) {
    return TokenError("expects string");
  }
  *result = lexer_.GetStrVal();
  lexer_.Lex();
  return true;
}

bool HloParser::ParseDxD(const string& name,
                         std::vector<tensorflow::int64>* result) {
  LocTy loc = lexer_.GetLoc();
  if (!result->empty()) {
    return Error(loc, StrFormat("sub-attribute '%s=' already exists", name));
  }
  // 1D
  if (lexer_.GetKind() == TokKind::kInt) {
    tensorflow::int64 number;
    if (!ParseInt64(&number)) {
      return Error(loc, StrFormat("expects sub-attribute '%s=i'", name));
    }
    result->push_back(number);
    return true;
  }
  // 2D or higher.
  if (lexer_.GetKind() == TokKind::kDxD) {
    string str = lexer_.GetStrVal();
    if (!SplitToInt64s(str, 'x', result)) {
      return Error(loc, StrFormat("expects sub-attribute '%s=ixj...'", name));
    }
    lexer_.Lex();
    return true;
  }
  return TokenError("expects token type kInt or kDxD");
}

bool HloParser::ParseWindowPad(
    std::vector<std::vector<tensorflow::int64>>* pad) {
  LocTy loc = lexer_.GetLoc();
  if (!pad->empty()) {
    return Error(loc, "sub-attribute 'pad=' already exists");
  }
  if (lexer_.GetKind() != TokKind::kPad) {
    return TokenError("expects window pad pattern, e.g., '0_0x3_3'");
  }
  string str = lexer_.GetStrVal();
  for (const auto& padding_dim_str : absl::StrSplit(str, 'x')) {
    std::vector<tensorflow::int64> low_high;
    if (!SplitToInt64s(padding_dim_str, '_', &low_high) ||
        low_high.size() != 2) {
      return Error(loc,
                   "expects padding_low and padding_high separated by '_'");
    }
    pad->push_back(low_high);
  }
  lexer_.Lex();
  return true;
}

// This is the inverse xla::ToString(PaddingConfig). The padding config string
// looks like "0_0_0x3_3_1". The string is first separated by 'x', each
// substring represents one PaddingConfigDimension. The substring is 3 (or 2)
// numbers joined by '_'.
bool HloParser::ParsePaddingConfig(PaddingConfig* padding) {
  if (lexer_.GetKind() != TokKind::kPad) {
    return TokenError("expects padding config, e.g., '0_0_0x3_3_1'");
  }
  LocTy loc = lexer_.GetLoc();
  string str = lexer_.GetStrVal();
  for (const auto& padding_dim_str : absl::StrSplit(str, 'x')) {
    std::vector<tensorflow::int64> padding_dim;
    if (!SplitToInt64s(padding_dim_str, '_', &padding_dim) ||
        (padding_dim.size() != 2 && padding_dim.size() != 3)) {
      return Error(loc,
                   "expects padding config pattern like 'low_high_interior' or "
                   "'low_high'");
    }
    auto* dim = padding->add_dimensions();
    dim->set_edge_padding_low(padding_dim[0]);
    dim->set_edge_padding_high(padding_dim[1]);
    dim->set_interior_padding(padding_dim.size() == 3 ? padding_dim[2] : 0);
  }
  lexer_.Lex();
  return true;
}

// '{' metadata_string '}'
bool HloParser::ParseMetadata(OpMetadata* metadata) {
  std::unordered_map<string, AttrConfig> attrs;
  optional<string> op_type;
  optional<string> op_name;
  optional<string> source_file;
  optional<tensorflow::int32> source_line;
  attrs["op_type"] = {/*required=*/false, AttrTy::kString, &op_type};
  attrs["op_name"] = {/*required=*/false, AttrTy::kString, &op_name};
  attrs["source_file"] = {/*required=*/false, AttrTy::kString, &source_file};
  attrs["source_line"] = {/*required=*/false, AttrTy::kInt32, &source_line};
  if (!ParseSubAttributes(attrs)) {
    return false;
  }
  if (op_type) {
    metadata->set_op_type(*op_type);
  }
  if (op_name) {
    metadata->set_op_name(*op_name);
  }
  if (source_file) {
    metadata->set_source_file(*source_file);
  }
  if (source_line) {
    metadata->set_source_line(*source_line);
  }
  return true;
}

bool HloParser::ParseOpcode(HloOpcode* result) {
  VLOG(1) << "ParseOpcode";
  if (lexer_.GetKind() != TokKind::kIdent) {
    return TokenError("expects opcode");
  }
  string val = lexer_.GetStrVal();
  auto status_or_result = StringToHloOpcode(val);
  if (!status_or_result.ok()) {
    return TokenError(StrFormat("expects opcode but sees: %s, error: %s", val,
                                status_or_result.status().error_message()));
  }
  *result = status_or_result.ValueOrDie();
  lexer_.Lex();
  return true;
}

bool HloParser::ParseFftType(FftType* result) {
  VLOG(1) << "ParseFftType";
  if (lexer_.GetKind() != TokKind::kIdent) {
    return TokenError("expects fft type");
  }
  string val = lexer_.GetStrVal();
  if (!FftType_Parse(val, result) || !FftType_IsValid(*result)) {
    return TokenError(StrFormat("expects fft type but sees: %s", val));
  }
  lexer_.Lex();
  return true;
}

bool HloParser::ParseFusionKind(HloInstruction::FusionKind* result) {
  VLOG(1) << "ParseFusionKind";
  if (lexer_.GetKind() != TokKind::kIdent) {
    return TokenError("expects fusion kind");
  }
  string val = lexer_.GetStrVal();
  auto status_or_result = StringToFusionKind(val);
  if (!status_or_result.ok()) {
    return TokenError(StrFormat("expects fusion kind but sees: %s, error: %s",
                                val,
                                status_or_result.status().error_message()));
  }
  *result = status_or_result.ValueOrDie();
  lexer_.Lex();
  return true;
}

bool HloParser::ParseRandomDistribution(RandomDistribution* result) {
  VLOG(1) << "ParseRandomDistribution";
  if (lexer_.GetKind() != TokKind::kIdent) {
    return TokenError("expects random distribution");
  }
  string val = lexer_.GetStrVal();
  auto status_or_result = StringToRandomDistribution(val);
  if (!status_or_result.ok()) {
    return TokenError(
        StrFormat("expects random distribution but sees: %s, error: %s", val,
                  status_or_result.status().error_message()));
  }
  *result = status_or_result.ValueOrDie();
  lexer_.Lex();
  return true;
}

bool HloParser::ParsePrecision(PrecisionConfig::Precision* result) {
  VLOG(1) << "ParsePrecision";
  if (lexer_.GetKind() != TokKind::kIdent) {
    return TokenError("expects random distribution");
  }
  string val = lexer_.GetStrVal();
  auto status_or_result = StringToPrecision(val);
  if (!status_or_result.ok()) {
    return TokenError(StrFormat("expects precision but sees: %s, error: %s",
                                val,
                                status_or_result.status().error_message()));
  }
  *result = status_or_result.ValueOrDie();
  lexer_.Lex();
  return true;
}

bool HloParser::ParseInt64(tensorflow::int64* result) {
  VLOG(1) << "ParseInt64";
  if (lexer_.GetKind() != TokKind::kInt) {
    return TokenError("expects integer");
  }
  *result = lexer_.GetInt64Val();
  lexer_.Lex();
  return true;
}

bool HloParser::ParseDouble(double* result) {
  switch (lexer_.GetKind()) {
    case TokKind::kDecimal:
      *result = lexer_.GetDecimalVal();
      break;
    case TokKind::kInt:
      *result = static_cast<double>(lexer_.GetInt64Val());
      break;
    case TokKind::kw_nan:
      *result = std::numeric_limits<double>::quiet_NaN();
      break;
    case TokKind::kw_inf:
      *result = std::numeric_limits<double>::infinity();
      break;
    case TokKind::kNegInf:
      *result = -std::numeric_limits<double>::infinity();
      break;
    default:
      return TokenError("expects decimal or integer");
  }
  lexer_.Lex();
  return true;
}

bool HloParser::ParseBool(bool* result) {
  if (lexer_.GetKind() != TokKind::kw_true &&
      lexer_.GetKind() != TokKind::kw_false) {
    return TokenError("expects true or false");
  }
  *result = lexer_.GetKind() == TokKind::kw_true;
  lexer_.Lex();
  return true;
}

bool HloParser::ParseToken(TokKind kind, const string& msg) {
  VLOG(1) << "ParseToken " << TokKindToString(kind) << " " << msg;
  if (lexer_.GetKind() != kind) {
    return TokenError(msg);
  }
  lexer_.Lex();
  return true;
}

bool HloParser::EatIfPresent(TokKind kind) {
  if (lexer_.GetKind() != kind) {
    return false;
  }
  lexer_.Lex();
  return true;
}

bool HloParser::AddInstruction(const string& name, HloInstruction* instruction,
                               LocTy name_loc) {
  auto result = current_name_table().insert({name, {instruction, name_loc}});
  if (!result.second) {
    Error(name_loc, StrCat("instruction already exists: ", name));
    return Error(/*loc=*/result.first->second.second,
                 "instruction previously defined here");
  }
  return true;
}

bool HloParser::AddComputation(const string& name, HloComputation* computation,
                               LocTy name_loc) {
  auto result = computation_pool_.insert({name, {computation, name_loc}});
  if (!result.second) {
    Error(name_loc, StrCat("computation already exists: ", name));
    return Error(/*loc=*/result.first->second.second,
                 "computation previously defined here");
  }
  return true;
}

StatusOr<HloSharding> HloParser::ParseShardingOnly() {
  lexer_.Lex();
  OpSharding op_sharding;
  if (!ParseSharding(&op_sharding)) {
    return InvalidArgument("Syntax error:\n%s", GetError());
  }
  if (lexer_.GetKind() != TokKind::kEof) {
    return InvalidArgument("Syntax error:\nExtra content after sharding");
  }
  return HloSharding::FromProto(op_sharding);
}

StatusOr<Window> HloParser::ParseWindowOnly() {
  lexer_.Lex();
  Window window;
  if (!ParseWindow(&window, /*expect_outer_curlies=*/false)) {
    return InvalidArgument("Syntax error:\n%s", GetError());
  }
  if (lexer_.GetKind() != TokKind::kEof) {
    return InvalidArgument("Syntax error:\nExtra content after window");
  }
  return window;
}

StatusOr<ConvolutionDimensionNumbers>
HloParser::ParseConvolutionDimensionNumbersOnly() {
  lexer_.Lex();
  ConvolutionDimensionNumbers dnums;
  if (!ParseConvolutionDimensionNumbers(&dnums)) {
    return InvalidArgument("Syntax error:\n%s", GetError());
  }
  if (lexer_.GetKind() != TokKind::kEof) {
    return InvalidArgument(
        "Syntax error:\nExtra content after convolution dnums");
  }
  return dnums;
}

StatusOr<PaddingConfig> HloParser::ParsePaddingConfigOnly() {
  lexer_.Lex();
  PaddingConfig padding_config;
  if (!ParsePaddingConfig(&padding_config)) {
    return InvalidArgument("Syntax error:\n%s", GetError());
  }
  if (lexer_.GetKind() != TokKind::kEof) {
    return InvalidArgument("Syntax error:\nExtra content after PaddingConfig");
  }
  return padding_config;
}

bool HloParser::ParseSingleInstruction(HloModule* module) {
  if (create_missing_instruction_ != nullptr || !scoped_name_tables_.empty()) {
    LOG(FATAL) << "Parser state is not clean. Please do not call any other "
                  "methods before calling ParseSingleInstruction.";
  }
  HloComputation::Builder builder(module->name());

  // The missing instruction hook we register creates the shaped instruction on
  // the fly as a parameter and returns it.
  int64 parameter_count = 0;
  create_missing_instruction_ =
      [this, &builder, &parameter_count](
          const string& name,
          const Shape& shape) -> std::pair<HloInstruction*, LocTy>* {
    string new_name = name.empty() ? StrCat("_", parameter_count) : name;
    HloInstruction* parameter = builder.AddInstruction(
        HloInstruction::CreateParameter(parameter_count++, shape, new_name));
    current_name_table()[new_name] = {parameter, lexer_.GetLoc()};
    return tensorflow::gtl::FindOrNull(current_name_table(), new_name);
  };

  // Parse the instruction with the registered hook.
  Scope scope(&scoped_name_tables_);
  if (CanBeShape()) {
    // This means that the instruction's left-hand side is probably omitted,
    // e.g.
    //
    //  f32[10] fusion(...), calls={...}
    if (!ParseInstructionRhs(&builder, module->name(), lexer_.GetLoc())) {
      return false;
    }
  } else {
    // This means that the instruction's left-hand side might exist, e.g.
    //
    //  foo = f32[10] fusion(...), calls={...}
    string root_name;
    if (!ParseInstruction(&builder, &root_name)) {
      return false;
    }
  }

  module->AddEntryComputation(builder.Build());
  for (auto& comp : computations_) {
    module->AddEmbeddedComputation(std::move(comp));
  }
  return true;
}

}  // namespace

StatusOr<std::unique_ptr<HloModule>> ParseHloString(
    absl::string_view str, const HloModuleConfig& config) {
  auto module = absl::make_unique<HloModule>(/*name=*/"_", config);
  HloParser parser(str);
  TF_RETURN_IF_ERROR(parser.Run(module.get()));
  return std::move(module);
}

StatusOr<std::unique_ptr<HloModule>> ParseHloString(absl::string_view str) {
  auto module = absl::make_unique<HloModule>(/*name=*/"_", HloModuleConfig());
  HloParser parser(str);
  TF_RETURN_IF_ERROR(parser.Run(module.get()));
  return std::move(module);
}

Status ParseHloString(absl::string_view str, HloModule* module) {
  TF_RET_CHECK(module->computation_count() == 0);
  HloParser parser(str);
  TF_RETURN_IF_ERROR(parser.Run(module));
  return Status::OK();
}

StatusOr<HloSharding> ParseSharding(absl::string_view str) {
  HloParser parser(str);
  return parser.ParseShardingOnly();
}

StatusOr<Window> ParseWindow(absl::string_view str) {
  HloParser parser(str);
  return parser.ParseWindowOnly();
}

StatusOr<ConvolutionDimensionNumbers> ParseConvolutionDimensionNumbers(
    absl::string_view str) {
  HloParser parser(str);
  return parser.ParseConvolutionDimensionNumbersOnly();
}

StatusOr<PaddingConfig> ParsePaddingConfig(absl::string_view str) {
  HloParser parser(str);
  return parser.ParsePaddingConfigOnly();
}

}  // namespace xla
