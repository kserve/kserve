# LoRA routing capacity fixtures

`lora-adapter-names-realistic-v1.txt` is the versioned realistic-v1 naming
fixture behind the published 100-adapter regex capacity claim. The claim is
bound to this exact name list: capacity depends on served-name lengths, so a
statement like "100 realistic adapters" is only reproducible against a pinned
fixture.

Name shape: `{team}-{task}-{lang}-v{1..4}` drawn deterministically from ten
teams, ten tasks, and ten language codes; every 7th name gets an HF-style
`acme/` org prefix and every 9th a `.r16` rank suffix, so the set exercises
`/` and `.` in served names. Index `i` (0..99) maps to
`teams[i%10]-tasks[(i/10)%10]-langs[(i*7)%10]-v{1+(i%4)}`.

`lora-regex-realistic-v1.golden` is the complete expected rendered pattern for
namespace `lora-capacity` and base model `llama-3-1-8b-instruct`. It is
produced by an independent generator (not the production renderer, to keep the
oracle non-circular): sort and deduplicate the names byte-wise, escape each
name and the namespace with `re.sub(r'([\\.+*?()|\[\]{}^$])', r'\\\1', name)`
(the regexp.QuoteMeta character set), and render
`^publishers/<ns>/models/(<base>|<name1>|...)$`.

Regenerating either file is an explicit, reviewed action - never done
implicitly by tests. Both files are reproducible from the description above
alone: the name list is checked in here, and the golden follows from sorting
and deduplicating it, escaping each name and the namespace, and rendering the
pattern. Nothing outside this repository is needed to review or rebuild them.

The e2e suite reads the name list from this file directly
(`test/e2e/llmisvc/fixtures.py`, `LORA_REALISTIC_ADAPTER_NAMES`), so the
adapters it deploys against a real gateway are these names; there is no second
copy to drift. It deploys a prefix rather than the whole list - one vLLM pod per
adapter-set is the expensive part of an e2e run, and the capacity claim itself
is what this golden pins. `E2E_LORA_ADAPTER_COUNT` sets how long that prefix is.
