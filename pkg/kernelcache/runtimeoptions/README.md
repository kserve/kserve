# Runtime option resolver

This package extracts explicitly registered runtime options from a Kubernetes
`corev1.Container`. It is used by KernelCache identity calculation and does
not execute commands or query Kubernetes resources.

## Resolution order

Values are resolved in this order. A later source replaces an earlier value:

```text
Env < Args < Command
```

Repeated options within one source use the last resolvable occurrence.

## Supported options

Only options registered in a `runtimeoptions.Spec` are considered. A spec maps
source-specific names to one canonical key:

```go
runtimeoptions.Spec{
    Key:     "dtype",
    Env:     []string{"VLLM_DTYPE"},
    Flags:   []string{"--dtype"},
    Boolean: false,
}
```

The following CLI forms are supported:

```text
--option=value
--option value
```

Boolean options support presence, explicit boolean values, and `--no-` forms:

```text
--enable-feature
--enable-feature=false
--no-enable-feature
```

Unknown options are ignored. An option value that starts with `-` is not
treated as a separate value.

## Environment variables

Direct `EnvVar.Value` values are supported. `ValueFrom` references are ignored
because this package does not read Secrets, ConfigMaps, or the Kubernetes API.

For shell commands, simple references can be expanded when their value is
available directly in the container environment:

```text
${VLLM_OPTIONS}
$VLLM_OPTIONS
```

For example, a direct value such as:

```text
VLLM_OPTIONS="--dtype float16 --tensor-parallel-size 4"
```

can contribute both registered options when used in a shell command:

```text
exec vllm serve ${VLLM_OPTIONS}
```

Unknown references remain unresolved and are not used as option values. A
reference inside single quotes is not expanded.

## Command handling

The resolver accepts already-tokenized direct commands and a small subset of
shell wrappers using `sh`, `bash`, `dash`, or `zsh` with `-c` (including common
combined flags such as `-lc`). The shell script must be present in the command
array, for example:

```yaml
command:
- /bin/bash
- -c
- exec vllm serve --dtype float16
```

For wrapper commands it can locate a final `exec ...` command, preserve
ordinary quoted arguments, and handle line continuations. The alternative form
where `command` ends at `-c` and the shell script is supplied as `args[0]` is
not supported.

This is intentionally not a Bash interpreter. The following are not executed
or evaluated:

- `${FOO:-default}` and other advanced parameter expansion
- `$(command)`, backticks, arithmetic expansion, and command substitution
- pipelines, redirects, command lists, and conditional shell execution
- variables assigned inside the shell script
- environment values supplied through `ValueFrom`

If the command contains unsupported or malformed shell syntax, the resolver
skips the command-derived values instead of guessing.

## Runtime profiles

The resolver is runtime-neutral. Each runtime can define its own `Spec` list.
The current KernelCache identity path builds a vLLM profile from its existing
factor definitions. Adding another runtime should add another profile and
should not require changing the generic resolver.
