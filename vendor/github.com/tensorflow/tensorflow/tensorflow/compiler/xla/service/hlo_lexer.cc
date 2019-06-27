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

#include "tensorflow/compiler/xla/service/hlo_lexer.h"

#include <unordered_map>

#include "absl/strings/escaping.h"
#include "absl/strings/numbers.h"
#include "absl/types/optional.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/statusor.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/core/lib/strings/numbers.h"
#include "tensorflow/core/platform/regexp.h"

namespace xla {
namespace {

using absl::string_view;

constexpr int kEOF = -1;
constexpr int kError = -2;

// [a-zA-Z0-9_.-]
bool IsIdentifierChar(char c) {
  return isalnum(static_cast<unsigned char>(c)) || c == '-' || c == '.' ||
         c == '_';
}

}  // namespace

int HloLexer::GetNextChar() {
  int current_char = PeekCurrentChar();
  if (current_char != kEOF && current_char != kError) {
    current_ptr_++;
  }
  return current_char;
}

int HloLexer::PeekCurrentChar() const {
  if (current_ptr_ == buf_.end()) {
    return kEOF;
  }
  char current_char = *current_ptr_;
  if (current_char == 0) {
    // '\0' should not appear in the middle of the string.
    return kError;
  }
  return static_cast<unsigned char>(current_char);
}

bool HloLexer::CanDereference(const char* ptr) const {
  return ptr < buf_.end() && ptr >= buf_.begin();
}

absl::string_view HloLexer::StringPieceFromPointers(const char* begin,
                                                    const char* end) const {
  CHECK(begin <= end);
  CHECK(begin == buf_.end() || CanDereference(begin));
  CHECK(end == buf_.end() || CanDereference(end));
  return absl::string_view(begin, end - begin);
}

tensorflow::RegexpStringPiece HloLexer::RegexpStringPieceFromPointers(
    const char* begin, const char* end) const {
  CHECK(begin <= end);
  CHECK(begin == buf_.end() || CanDereference(begin));
  CHECK(end == buf_.end() || CanDereference(end));
  return tensorflow::RegexpStringPiece(begin, end - begin);
}

TokKind HloLexer::LexToken() {
  while (true) {
    token_start_ = current_ptr_;

    int current_char = GetNextChar();
    switch (current_char) {
      default:
        // [a-zA-Z_]
        if (isalpha(static_cast<unsigned char>(current_char)) ||
            current_char == '_') {
          return LexIdentifier();
        }
        return TokKind::kError;
      case kEOF:
        // Hit the end of the input buffer.
        return TokKind::kEof;
      case kError:
        // Hit an invalid character in the input buffer.
        return TokKind::kError;
      case ' ':
      case '\t':
      case '\n':
      case '\r':
        // Ignore whitespace.
        continue;
      case '0':
      case '1':
      case '2':
      case '3':
      case '4':
      case '5':
      case '6':
      case '7':
      case '8':
      case '9':
      case '-':
        if (current_char == '-' && PeekCurrentChar() == '>') {
          current_ptr_++;
          return TokKind::kArrow;
        }
        return LexNumberOrPattern();
      case '=':
        return TokKind::kEqual;
      case ',':
        return TokKind::kComma;
      case '%':
        return LexPercent();
      case ':':
        return TokKind::kColon;
      case '[':
        return TokKind::kLsquare;
      case ']':
        return TokKind::kRsquare;
      case '{':
        return TokKind::kLbrace;
      case '}':
        return TokKind::kRbrace;
      case '(':
        return TokKind::kLparen;
      case ')':
        return TokKind::kRparen;
      case '/': {
        if (PeekCurrentChar() == '*') {
          // This is the start of a /*...*/ delimited comment. Save the current
          // location in case the comment is unterminated so the error message
          // will point to the beginning of the comment.
          const char* comment_start = current_ptr_;
          current_ptr_++;
          // Advance until '*/' is found.
          while (true) {
            int current = GetNextChar();
            if (current == '*' && PeekCurrentChar() == '/') {
              // End of comment.
              current_ptr_++;
              break;
            }
            if (current == kEOF) {
              // Unterminated comment.
              current_ptr_ = comment_start;
              return TokKind::kError;
            }
            if (current == kError) {
              return TokKind::kError;
            }
          }
          // Return no token for the comment. Keep lexing.
          continue;
        } else if (PeekCurrentChar() == '/') {
          // This is the start of a '//' delimited comment. Throw away
          // everything until end of line or file. The end-of-line character(s)
          // are left unlexed in the buffer which is harmless because these are
          // skipped later by the lexer. This approach enables support for
          // different end-of-line encodings.
          while (true) {
            int current = PeekCurrentChar();
            if (current == kEOF || current == '\n' || current == '\r') {
              break;
            }
            if (current == kError) {
              return TokKind::kError;
            }
            current_ptr_++;
          }
          continue;
        }
        // A lone '/' is an error.
        return TokKind::kError;
      }
      case '"':
        return LexString();
    }
  }
}

// Lex a shape, name, keyword, attribute name, the dim labels pattern, and
// other identifiers.
//
// shape    ::= ([a-zA-Z0-9_]*[0-9]*)\[([0-9,]*)\](?:\s*{([0-9,]*)})?
// name     ::= [a-zA-Z_][a-zA-Z0-9_.-]*:
// keyword  ::= HloModule, ENTRY, ...
// attribute_name ::= condition, body, dimensions, ...
// dim_labels_pattern ::= [0-9bf]{2,}_[0-9io]{2,}->[0-9bf]{2,}
// identifiers ::= other cases that match [a-zA-Z_][a-zA-Z0-9_.-]*
TokKind HloLexer::LexIdentifier() {
  {
    auto consumable = RegexpStringPieceFromPointers(token_start_, buf_.end());
    // 'consumable' will be advanced iff its prefix matches the pattern.
    static LazyRE2 shape_pattern = {
        R"(^(\w*\d*)\[([\d,\s]*)\](?:(dense|sparse)?{([\d,\s]+)})?)"};
    if (RE2::Consume(&consumable, *shape_pattern)) {
      auto status_or_shape = ShapeUtil::ParseShapeString(
          StringPieceFromPointers(token_start_, consumable.begin()));
      if (status_or_shape.ok()) {
        // This is a shape string.
        shape_val_ = status_or_shape.ValueOrDie();
        current_ptr_ = consumable.begin();
        return TokKind::kShape;
      }
    }
  }

  while (IsIdentifierChar(PeekCurrentChar())) {
    current_ptr_++;
  }

  // If followed by ':', it's a name.
  if (PeekCurrentChar() == ':') {
    str_val_.assign(token_start_, current_ptr_);
    current_ptr_++;  // skip ':'
    return TokKind::kName;
  }

  // If followed by '=', it's a attribute name.
  if (PeekCurrentChar() == '=') {
    str_val_.assign(token_start_, current_ptr_);
    current_ptr_++;  // skip '='
    return TokKind::kAttributeName;
  }

  absl::string_view identifier =
      StringPieceFromPointers(token_start_, current_ptr_);

  // See if this is a keyword.
#define KEYWORD(STR)            \
  do {                          \
    if (identifier == #STR) {   \
      return TokKind::kw_##STR; \
    }                           \
  } while (false)

  KEYWORD(true);
  KEYWORD(false);
  KEYWORD(inf);
  KEYWORD(nan);
  KEYWORD(HloModule);
  KEYWORD(ENTRY);
  KEYWORD(ROOT);
  KEYWORD(maximal);
  KEYWORD(replicated);

#undef KEYWORD

  {
    auto consumable = RegexpStringPieceFromPointers(token_start_, buf_.end());
    static LazyRE2 dim_labels_pattern = {
        R"([0-9bf]{2,}_[0-9io]{2,}->[0-9bf]{2,})"};
    if (RE2::Consume(&consumable, *dim_labels_pattern)) {
      current_ptr_ = consumable.begin();
      str_val_.assign(token_start_, current_ptr_);
      return TokKind::kDimLabels;
    }
  }

  str_val_ = string(identifier);
  return TokKind::kIdent;
}

// Lex names after a % character.
// name ::= [a-zA-Z_][a-zA-Z0-9_.-]*
TokKind HloLexer::LexPercent() {
  const char* name_start = current_ptr_;
  if (isalpha(static_cast<unsigned char>(PeekCurrentChar())) ||
      PeekCurrentChar() == '_') {
    current_ptr_++;
    while (IsIdentifierChar(PeekCurrentChar())) {
      current_ptr_++;
    }
    str_val_.assign(name_start, current_ptr_);
    return TokKind::kName;
  }
  return TokKind::kError;
}

// Lex integer and floating-point values, -inf, and patterns for dim labels,
// dxd (e.g. 1x2x3), and pad.
//
// fp with exp ::= [-]?([0-9]+|[0-9]+[.][0-9]*|[0-9]*[.][0-9]+)([eE][+-]?[0-9]+)
// fp without exp ::= [-]?([0-9]+[.][0-9]*|[0-9]*[.][0-9]+)
// dim_labels_pattern ::= [0-9bf]{2,}_[0-9io]{2,}->[0-9bf]{2,}
// dxd_pattern ::= [0-9]+(x[0-9]+)+
// pad_pattern ::=
//   [-]?[0-9]+_[-]?[0-9]+(_[0-9]+)?(x[-]?[0-9]+_[-]?[0-9]+(_[0-9]+)?)*
// int ::=  [-]?[0-9]+
// negative inf ::= '-inf'
TokKind HloLexer::LexNumberOrPattern() {
  auto consumable = RegexpStringPieceFromPointers(token_start_, buf_.end());
  static LazyRE2 float_pattern = {
      R"([-]?((\d+|\d+[.]\d*|\d*[.]\d+)([eE][+-]?\d+))|[-]?(\d+[.]\d*|\d*[.]\d+))"};
  if (RE2::Consume(&consumable, *float_pattern)) {
    current_ptr_ = consumable.begin();
    CHECK(absl::SimpleAtod(string(token_start_, current_ptr_), &decimal_val_));
    return TokKind::kDecimal;
  }

  static LazyRE2 dim_labels_pattern = {
      R"([0-9bf]{2,}_[0-9io]{2,}->[0-9bf]{2,})"};
  static LazyRE2 dxd_pattern = {R"([0-9]+(x[0-9]+)+)"};
  static LazyRE2 pad_pattern = {
      R"([-]?[0-9]+_[-]?[0-9]+(_[0-9]+)?(x[-]?[0-9]+_[-]?[0-9]+(_[0-9]+)?)*)"};

  if (RE2::Consume(&consumable, *dim_labels_pattern)) {
    current_ptr_ = consumable.begin();
    str_val_.assign(token_start_, current_ptr_);
    return TokKind::kDimLabels;
  }

  if (RE2::Consume(&consumable, *dxd_pattern)) {
    current_ptr_ = consumable.begin();
    str_val_.assign(token_start_, current_ptr_);
    return TokKind::kDxD;
  }

  if (RE2::Consume(&consumable, *pad_pattern)) {
    current_ptr_ = consumable.begin();
    str_val_.assign(token_start_, current_ptr_);
    return TokKind::kPad;
  }

  static LazyRE2 int_pattern = {R"([-]?\d+)"};
  if (RE2::Consume(&consumable, *int_pattern)) {
    current_ptr_ = consumable.begin();
    auto slice = StringPieceFromPointers(token_start_, current_ptr_);
    if (absl::SimpleAtoi(slice, &int64_val_)) {
      return TokKind::kInt;
    }
    LOG(ERROR) << "Failed to parse int literal: " << slice;
    return TokKind::kError;
  }

  static LazyRE2 neg_inf = {"-inf"};
  if (RE2::Consume(&consumable, *neg_inf)) {
    current_ptr_ = consumable.begin();
    return TokKind::kNegInf;
  }

  return TokKind::kError;
}

std::pair<unsigned, unsigned> HloLexer::GetLineAndColumn(LocTy location) const {
  unsigned line_no = 1;
  const char* start = buf_.begin();
  const char* ptr = start;
  if (line_no_cache_.last_query && CanDereference(line_no_cache_.last_query) &&
      line_no_cache_.last_query <= location) {
    ptr = line_no_cache_.last_query;
    line_no = line_no_cache_.line_no_of_query;
  }
  for (; ptr != location; ptr++) {
    CHECK_LT(ptr, buf_.end());
    if (*ptr == '\n') {
      line_no++;
    }
  }

  // Update the line number cache.
  line_no_cache_.last_query = ptr;
  line_no_cache_.line_no_of_query = line_no;
  size_t line_offset = StringPieceFromPointers(start, ptr).rfind('\n');
  if (line_offset == absl::string_view::npos) {
    line_offset = 0;
  }
  return {line_no, ptr - start - line_offset};
}

absl::string_view HloLexer::GetLine(LocTy loc) const {
  if (!CanDereference(loc)) {
    return "LINE OUT OF RANGE";
  }
  size_t line_start =
      StringPieceFromPointers(buf_.begin(), loc + 1).rfind('\n');
  const char* start = line_start == absl::string_view::npos
                          ? buf_.begin()
                          : buf_.begin() + line_start + 1;
  size_t line_end = StringPieceFromPointers(loc, buf_.end()).find('\n');
  const char* end =
      line_end == absl::string_view::npos ? buf_.end() : loc + line_end;

  return StringPieceFromPointers(start, end);
}

// Lexes quoted string with escaping characters. If matched, the quoted string
// will be unescaped and stored to str_val_.
TokKind HloLexer::LexString() {
  auto consumable = RegexpStringPieceFromPointers(token_start_, buf_.end());
  static LazyRE2 escaping_pattern = {R"("([^"\\]|\\.)*")"};
  if (RE2::Consume(&consumable, *escaping_pattern)) {
    current_ptr_ = consumable.begin();
    absl::string_view raw =
        StringPieceFromPointers(token_start_ + 1, current_ptr_ - 1);
    string error;
    if (!absl::CUnescape(raw, &str_val_, &error)) {
      LOG(ERROR) << "Failed unescaping string: " << raw << ". error: " << error;
      return TokKind::kError;
    }
    return TokKind::kString;
  }
  return TokKind::kError;
}

string TokKindToString(TokKind kind) {
  switch (kind) {
    case TokKind::kEof:
      return "kEof";
    case TokKind::kError:
      return "kError";
    case TokKind::kEqual:
      return "kEqaul";
    case TokKind::kComma:
      return "kComma";
    case TokKind::kColon:
      return "kColon";
    case TokKind::kLsquare:
      return "kLsquare";
    case TokKind::kRsquare:
      return "kRsquare";
    case TokKind::kLbrace:
      return "kLbrace";
    case TokKind::kRbrace:
      return "kRbrace";
    case TokKind::kLparen:
      return "kLparen";
    case TokKind::kRparen:
      return "kRparen";
    case TokKind::kArrow:
      return "kArrow";
    case TokKind::kw_HloModule:
      return "kw_HloModule";
    case TokKind::kw_ENTRY:
      return "kw_ENTRY";
    case TokKind::kw_ROOT:
      return "kw_ROOT";
    case TokKind::kw_true:
      return "kw_true";
    case TokKind::kw_false:
      return "kw_false";
    case TokKind::kw_maximal:
      return "kw_maximal";
    case TokKind::kw_replicated:
      return "kw_replicated";
    case TokKind::kw_nan:
      return "kw_nan";
    case TokKind::kw_inf:
      return "kw_inf";
    case TokKind::kNegInf:
      return "kNegInf";
    case TokKind::kName:
      return "kName";
    case TokKind::kAttributeName:
      return "kAttributeName";
    case TokKind::kDimLabels:
      return "kDimLabels";
    case TokKind::kDxD:
      return "kDxD";
    case TokKind::kPad:
      return "kPad";
    case TokKind::kIdent:
      return "kIdent";
    case TokKind::kString:
      return "kString";
    case TokKind::kShape:
      return "kShape";
    case TokKind::kInt:
      return "kInt";
    case TokKind::kDecimal:
      return "kDecimal";
  }
}

}  // namespace xla
