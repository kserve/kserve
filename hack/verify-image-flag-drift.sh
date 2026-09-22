#!/bin/bash

# Copyright 2026 The KServe Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Detects CLI flag drift when a container image tag is bumped.
#
# The flags our manifests render are a contract with a binary we do not build.
# When upstream retires one, nothing here fails until the pod crash-loops on a
# cluster - often only under a config CI never exercises, like TLS or a GPU.
#
# For every image whose tag changed against BASE_REF this checks that
#   1. the new tag exists in the registry,
#   2. an app.kubernetes.io/version annotation governing that image still
#      tracks the tag - the llmisvc controller gates its flag migrations on
#      that annotation rather than on the image, so a stale one leaves the
#      migrations dormant against a binary that needs them,
#   3. the new binary still accepts every flag our manifests pass it.
#
# Checks 1 and 2 fail the build, and only on an unambiguous answer. They differ
# deliberately when the answer cannot be had: an unparseable manifest blocks,
# because the annotation check is required and an unreadable file leaves it
# unperformed, while a registry that will not answer does not, because it says
# nothing about the tag either way.
#
# Check 3 is advisory: it reads what the manifests declare rather than what the
# controller finally renders, so a flag stripped by a version-gated migration
# would report as a false failure. That is the one prerequisite for promoting
# it with REPLAY_BLOCKING=1.
#
# Out of scope: images pinned across split image/tag chart values, and flags
# passed through an inline shell script, where the entrypoint is a shell.
#
# Usage: BASE_REF=origin/master ENGINE=docker hack/verify-image-flag-drift.sh
#        BASE_REF=<sha>^ HEAD_REF=<sha> ... to replay a past bump

set -euo pipefail

BASE_REF=${BASE_REF:-origin/master}
HEAD_REF=${HEAD_REF:-HEAD}
ENGINE=${ENGINE:-docker}
YQ=${YQ:-yq}
REPLAY_BLOCKING=${REPLAY_BLOCKING:-0}
VERSION_ANNOTATION="app.kubernetes.io/version"
SCAN_PATHS=(config charts docs/samples test)

CHANGED_FILES=""
blocking=0 # fails the build
advisory=0 # reported, does not fail the build

# A tooling failure must never read as "no drift".
die() {
    echo "verify-image-flag-drift: $*" >&2
    exit 1
}

not_in() {
    comm -23 <(echo "$1") <(echo "$2")
}


# --------------------------------------------------------------------------
# Image references
# --------------------------------------------------------------------------

# "<repo>\t<version>", where version is whatever identifies the build: a tag, a
# digest, or both. Splitting on the last colon instead puts the digest algorithm
# into the repository name, so repo:v1@sha256:aaa and repo:v2@sha256:bbb look
# like unrelated repositories and a bump between them reads as a new image.
split_image() {
    local image=$1 ref=$1 digest="" repo tag=""
    [[ $image == *@* ]] && {
        ref=${image%%@*}
        digest=${image#*@}
    }
    # A colon in the final path segment is a tag; earlier it is a registry port.
    if [[ ${ref##*/} == *:* ]]; then
        repo=${ref%:*}
        tag=${ref##*:}
    else
        repo=$ref
    fi
    printf '%s\t%s\n' "$repo" "${tag}${digest:+@${digest}}"
}

image_ref() {
    local repo=$1 version=$2
    [[ $version == @* ]] && {
        echo "${repo}${version}"
        return
    }
    echo "${repo}:${version}"
}

version_label() {
    local version=$1
    [[ $version == @* ]] && {
        echo "${version:1:14}…"
        return
    }
    echo "${version%%@*}"
}

# Docker resolves bare names to Docker Hub implicitly, podman refuses them, and
# the manifests use both styles. Qualifying keeps the two engines agreeing.
qualify() {
    local image=$1 first=${1%%/*}
    [[ $image == */* && ($first == *.* || $first == *:* || $first == localhost) ]] && {
        echo "$image"
        return
    }
    [[ $image == */* ]] && echo "docker.io/${image}" || echo "docker.io/library/${image}"
}

# "<file>\t<repo>\t<version>" for every concrete reference at the given ref.
# Locations are kept rather than a flat set: a repository can carry different
# tags in different files, and a set difference over the whole tree would hide a
# bump landing on a tag already present elsewhere.
image_locations() {
    local ref=$1 hits rc=0 line file image
    hits=$(git grep -I -E '^[[:space:]]*-?[[:space:]]*image:[[:space:]]' "$ref" -- "${SCAN_PATHS[@]}") || rc=$?
    # git grep exits 1 for "no matches" and 128 for a real failure - a bad ref, a
    # shallow clone, a broken checkout. Swallowing those turns an unverified run
    # into a green "no image tag changes".
    ((rc > 1)) && die "git grep failed for ref '${ref}' (exit ${rc}); cannot read image references"
    while IFS= read -r line; do
        line=${line#"$ref":} # git grep prefixes each hit with the rev
        file=${line%%:*}
        # Manifests only: the same pattern matches prose in READMEs and ko
        # configmaps, where docs/samples carries `{username}/torchserve:latest`.
        [[ $file =~ \.ya?ml$ ]] || continue
        image=$(sed -E 's/.*image:[[:space:]]*"?([^"[:space:]]+).*/\1/' <<<"${line#*:}")
        [[ $image == *'{{'* || $image == *'$'* || $image == *'{'* ]] && continue
        [[ ${image##*/} == *:* || $image == *@* ]] || continue
        printf '%s\t%s\n' "$file" "$(split_image "$image")"
    done <<<"$hits" | sort -u
}

# --------------------------------------------------------------------------
# Reading manifests
# --------------------------------------------------------------------------

strip_template() {
    sed -E 's/\{\{[^}]*\}\}//g' <<<"$1" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g'
}

# "<argv prefix>\t<flag>" for every flag the manifests pass to the image, paired
# with the command it actually reaches. The prefix is the leading run of non-flag
# entries in `command`, so a subcommand-scoped flag and a container overriding
# the image entrypoint are both judged against the right binary.
#
# Only `command` contributes the prefix. A container inheriting the entrypoint
# and putting a subcommand in `args` would replay bare; nothing does that today,
# and the failure mode is a missed finding rather than a false one.
#
# The yq groups are concatenated into one array before streaming: as separate
# comma branches yq drops the later ones when an earlier is empty, silently
# losing every flag from a container with args but no command.
contexts_for() {
    local repo=$1 file line kind value blob=""
    shift
    for file in "$@"; do
        local -a cmd=() args=()
        while IFS= read -r line; do
            kind=${line:0:1}
            value=${line:1}
            case $kind in
            C) cmd+=("$value") ;;
            A) args+=("$value") ;;
            S) blob="$value" ;;
            E)
                SCRIPT_BODY="$blob" \
                    emit_context "${#cmd[@]}" "${cmd[@]+"${cmd[@]}"}" -- "${args[@]+"${args[@]}"}"
                cmd=()
                args=()
                blob=""
                ;;
            esac
        done < <(git show "${HEAD_REF}:${file}" 2>/dev/null | "$YQ" -N -r '
            .. | select(has("image")) | select(.image == "'"$repo"':*") as $c
            | ( ($c.command // [] | map(select(test("\n") | not) | "C" + .))
              + ($c.args    // [] | map(select(test("\n") | not) | "A" + .))
              + (($c.command // []) + ($c.args // []) | map(select(test("\n")) | "S" + sub("\n"; " ")))
              + ["E"] )[]
        ' - 2>/dev/null || true)
    done | sort -u
}

# Split one container's argv into prefix and flags. A shell entrypoint cannot be
# replayed, so it yields "!SHELL\t<what it launches>\t<flags behind it>" - both
# derived from the script rather than declared in YAML, since a marker could
# drift from the code and produce a confident wrong answer.
emit_context() {
    local ncmd=$1 i token bare
    shift
    local -a argv=("$@") lead=()
    for ((i = 0; i < ncmd; i++)); do
        bare=$(strip_template "${argv[i]}")
        [[ $bare == -* ]] && break
        lead+=("${argv[i]}")
    done

    if [[ ${#lead[@]} -gt 0 && ${lead[0]##*/} =~ ^(sh|bash|ash|dash|zsh)$ ]]; then
        local launched count
        launched=$(grep -oE '\bexec [A-Za-z0-9_./-]+( [a-z][a-z0-9-]+)*' <<<"$SCRIPT_BODY" |
            head -1 | sed 's/^exec //')
        count=$(grep -oE -- '--[A-Za-z][A-Za-z0-9._-]*' <<<"$SCRIPT_BODY" | sort -u | wc -l)
        printf '!SHELL\t%s\t%s\n' "${launched:-}" "${count:-0}"
        return 0
    fi

    local joined="${lead[*]}"
    for token in "${argv[@]}"; do
        [[ $token == "--" ]] && continue
        bare=$(strip_template "$token")
        [[ $bare == --[A-Za-z]* ]] || continue
        printf '%s\t%s\n' "$joined" "$(grep -oE -- '--[A-Za-z][A-Za-z0-9._-]*[A-Za-z0-9]' <<<"$bare" | head -1)"
    done
}

# "<declared version>\t<file>" for annotations governing the image at that exact
# tag, within the given files. Both halves of the scoping matter: the file list
# confines the check to where the bump landed, and the exact repo:tag confines it
# to the node that moved, since one file can pin a repository at two tags.
#
# An unparseable file yields "!ERROR\t<file>" rather than nothing, so a blocking
# check never passes because it could not look. The query ends in an array tied
# to the matched node, not a bare $v: a lone variable reference still emits when
# the select matched nothing.
declared_versions_for() {
    local repo=$1 tag=$2 file out rc
    shift 2
    for file in "$@"; do
        rc=0
        out=$(git show "${HEAD_REF}:${file}" 2>/dev/null | "$YQ" -N -r '
            .. | select(has("annotations"))
            | select(.annotations | has("'"$VERSION_ANNOTATION"'"))
            | .annotations["'"$VERSION_ANNOTATION"'"] as $v
            | (.. | select(has("image")) | select(.image == "'"$repo"':'"$tag"'"))
            | [$v, .image] | @tsv
        ' - 2>&1) || rc=$?
        if ((rc != 0)); then
            log "yq failed on ${file} (exit ${rc}): ${out//$'\n'/ }"
            printf '!ERROR\t%s\n' "$file"
            continue
        fi
        awk -F'\t' -v f="$file" 'NF { print $1 "\t" f }' <<<"$out"
    done | sort -u
}

# Every manifest line passing a flag, marked when this diff does not touch it.
# The flag's own line is never the image line a finding is anchored to, and a
# deprecated flag is rarely confined to the files being bumped.
flag_sites() {
    local flag=$1 hit file
    while IFS= read -r hit; do
        [[ -z $hit ]] && continue
        file=${hit%%:*}
        if [[ $'\n'${CHANGED_FILES}$'\n' == *$'\n'"${file}"$'\n'* ]]; then
            echo "  - \`${hit}\`"
        else
            echo "  - \`${hit}\` (not in this diff)"
        fi
    done < <(git grep -n -F -- "$flag" "$HEAD_REF" -- "${SCAN_PATHS[@]}" 2>/dev/null |
        sed "s|^${HEAD_REF}:||" | grep -E '\.ya?ml:' | cut -d: -f1,2 | sort -u || true)
}

# --------------------------------------------------------------------------
# Probing images
# --------------------------------------------------------------------------

# Run an image under the argv prefix its manifest uses. Always succeeds; callers
# judge by the output.
run_under() {
    local image=$1 prefix=$2 secs=$3
    shift 3
    local container="flagdrift-$$-$RANDOM"
    local -a argv=(run --rm --name "$container" --network=none)
    if [[ -n $prefix ]]; then
        local -a pre
        read -r -a pre <<<"$prefix"
        argv+=(--entrypoint "${pre[0]}" "$image" "${pre[@]:1}" "$@")
    else
        argv+=("$image" "$@")
    fi
    timeout "$secs" "$ENGINE" "${argv[@]}" 2>&1 || true
    "$ENGINE" rm -f "$container" >/dev/null 2>&1 || true
}

# Flags the image documents in --help. Never fails: a binary that ignores --help
# and starts serving is killed by the timeout, and unmatched help leaves the
# greps empty - under `set -e` either would abort the run at the call site.
help_flags() {
    local out=""
    out=$(run_under "$1" "$2" 30 --help) || true
    grep -oE '^[[:space:]]+(-[A-Za-z], )?--[A-Za-z][A-Za-z0-9._-]*' <<<"$out" |
        grep -oE -- '--[A-Za-z][A-Za-z0-9._-]*' | sort -u || true
}

# "present", "missing" or "unknown". Only an explicit not-found counts as
# missing: auth, throttling and network failures exit the same way as a deleted
# tag, and calling those missing would block a pull request over a DNS blip.
tag_status() {
    local image=$1 out="" attempt delay
    for attempt in 1 2 3; do
        if out=$("$ENGINE" manifest inspect "$image" 2>&1); then
            echo "present"
            return
        fi
        if grep -qiE 'manifest unknown|not found|MANIFEST_UNKNOWN|404' <<<"$out"; then
            echo "missing"
            return
        fi
        delay=$((attempt * 3))
        log "registry lookup for ${image} failed (attempt ${attempt}/3):"
        log "  ${out//$'\n'/ }"
        [[ $attempt -lt 3 ]] && log "  retrying in ${delay}s" && sleep "$delay"
    done
    echo "unknown"
}

# "ok", "rejected", "unknown", or "deprecated|<the parser's own message>".
#
# Trailing --help usually makes the binary print usage and exit, but a
# value-taking flag swallows it and boots the server. That is fine: unknown-flag
# errors and deprecation notices are emitted while parsing, so the timeout only
# bounds the wait. A container that dies before parsing prints no verdict, which
# would otherwise read as "flag accepted".
probe() {
    local image=$1 prefix=$2 flag=$3 name=${3#--} out
    out=$(run_under "$image" "$prefix" 15 "$flag" --help) || true
    if grep -qE 'fatal error: newosproc|runtime: failed to create|cannot connect to the Docker daemon|OCI runtime|no such image|error creating container' <<<"$out"; then
        echo "unknown"
        return
    fi
    if grep -qE "(unknown flag|unknown option|flag provided but not defined|unrecognized arguments?):? *-{1,2}${name}\b" <<<"$out"; then
        echo "rejected"
    elif grep -qE "[Ff]lag ${flag} has been deprecated" <<<"$out"; then
        echo "deprecated|$(grep -oE "[Ff]lag ${flag} has been deprecated.*" <<<"$out" | head -1)"
    else
        echo "ok"
    fi
}

# --------------------------------------------------------------------------
# Reporting to GitHub
#
# Annotations need no token, which keeps them working for pull requests from
# forks. They go to fd 3 (the real stdout) so they stay out of the job summary,
# and out of the stdout of any function whose output is being captured.
# --------------------------------------------------------------------------

log() {
    echo "$*" >&3
}

# Workflow commands are single-line, so newlines travel as %0A. Percent is
# escaped first or it would mangle the escapes that follow.
escape_annotation() {
    local s=${1//'%'/%25}
    s=${s//$'\r'/%0D}
    printf '%s' "${s//$'\n'/%0A}"
}

annotate() {
    [[ -n ${GITHUB_ACTIONS:-} ]] || return 0
    local level=$1 image=$2 message=$3 file line
    message=$(escape_annotation "$message")
    while IFS=: read -r file line _; do
        [[ $file =~ \.ya?ml$ ]] || continue
        echo "::${level} file=${file},line=${line},title=Image CLI flag drift::${message}" >&3
    done < <(git grep -n -F -- "$image" "$HEAD_REF" -- "${SCAN_PATHS[@]}" |
        sed "s|^${HEAD_REF}:||" || true)
}

# Same, pinned to the first line of one file matching a pattern.
annotate_line() {
    [[ -n ${GITHUB_ACTIONS:-} ]] || return 0
    local level=$1 file=$2 pattern=$3 message=$4 line
    message=$(escape_annotation "$message")
    line=$(git show "${HEAD_REF}:${file}" 2>/dev/null | grep -n -F -m1 -- "$pattern" | cut -d: -f1)
    echo "::${level} file=${file},line=${line:-1},title=Image CLI flag drift::${message}" >&3
}

# flag_sites flattened into an annotation body, so the reviewer never has to
# open the job log to find out which lines to edit.
flag_sites_inline() {
    local sites
    sites=$(flag_sites "$1" | sed 's/^  - /- /; s/`//g')
    [[ -z $sites ]] && return 0
    printf '\n\nPassed at:\n%s' "$sites"
}

# --------------------------------------------------------------------------
# Checking one bumped image
# --------------------------------------------------------------------------

check_image() {
    local repo=$1 old_tag=$2 new_tag=$3 files=$4
    local image label_new label_old
    image=$(image_ref "$repo" "$new_tag")
    label_new=$(version_label "$new_tag")
    label_old=$(version_label "$old_tag")

    echo "### ${repo}"
    echo
    echo "\`${label_old:-not referenced before}\` -> \`${label_new}\`"
    echo

    # Reads only the manifests, so it runs before the registry is consulted.
    # Ordering it after would let an outage skip a required, fail-closed check
    # that never needed the network.
    local declared file
    # shellcheck disable=SC2086 # files is a deliberately word-split list
    while IFS=$'\t' read -r declared file; do
        if [[ $declared == "!ERROR" ]]; then
            echo "- **FAIL**: could not parse \`${file}\`, so its" \
                "\`${VERSION_ANNOTATION}\` could not be checked against \`${label_new}\`." \
                "See the job log."
            annotate_line error "$file" "image:" \
                "this file pins ${label_new} but could not be parsed, so its ${VERSION_ANNOTATION} was never checked"
            blocking=$((blocking + 1))
            continue
        fi
        [[ ${declared#v} == "${label_new#v}" ]] && continue
        echo "- **FAIL**: \`${VERSION_ANNOTATION}: ${declared}\` in \`${file}\` no longer matches" \
            "\`${label_new}\`, so version-gated migrations keyed on it stay dormant"
        annotate_line error "$file" "$VERSION_ANNOTATION" \
            "still declares ${declared} while the image moved to ${label_new}, so version-gated migrations keyed on this annotation stay dormant"
        blocking=$((blocking + 1))
    done < <(declared_versions_for "$repo" "$new_tag" $files)

    local qimage lookup
    qimage=$(qualify "$image")
    lookup=$(tag_status "$qimage")
    case $lookup in
    missing)
        echo "- **FAIL**: tag \`${label_new}\` does not exist in the registry"
        echo
        annotate error "$image" "tag ${label_new} does not exist in the registry"
        blocking=$((blocking + 1))
        return
        ;;
    unknown)
        echo "- **SKIPPED**: registry lookup for \`${label_new}\` failed after 3 attempts;" \
            "its flags were not replayed. See the job log for the errors."
        echo
        annotate warning "$image" "registry lookup failed after 3 attempts, so ${label_new} was not verified"
        advisory=$((advisory + 1))
        return
        ;;
    esac

    local -a contexts=() all=()
    # shellcheck disable=SC2086 # files is a deliberately word-split list
    mapfile -t all < <(contexts_for "$repo" $files)

    local shell_wrapped=0 launched="" unchecked=0 row rest
    for row in "${all[@]+"${all[@]}"}"; do
        if [[ $row == '!SHELL'* ]]; then
            shell_wrapped=1
            rest=${row#*$'\t'}
            [[ -z $launched ]] && launched=${rest%%$'\t'*}
            ((${rest##*$'\t'} > unchecked)) && unchecked=${rest##*$'\t'}
        else
            contexts+=("$row")
        fi
    done

    # Permanent rather than transient, so it shows on every bump of these
    # images - which is the honest signal: those flags are verified by nothing.
    if [[ $shell_wrapped -eq 1 ]]; then
        echo "- **SKIPPED**: the manifests launch this image through an inline shell" \
            "script${launched:+, which runs \`${launched}\`}, so the container entrypoint" \
            "is a shell rather than the binary and its" \
            "${unchecked:+${unchecked} }flag(s) were not replayed"
        advisory=$((advisory + 1))
    fi

    if [[ ${#contexts[@]} -eq 0 ]]; then
        [[ $shell_wrapped -eq 0 ]] &&
            echo "- note: the manifests pass no CLI flags to this image"
        echo
        return
    fi

    if ! "$ENGINE" pull -q "$qimage" >/dev/null 2>&1; then
        echo "- **SKIPPED**: could not pull \`${image}\`; flags not replayed"
        echo
        advisory=$((advisory + 1))
        return
    fi

    local qold=""
    [[ -n $old_tag ]] && qold=$(qualify "$(image_ref "$repo" "$old_tag")")
    if [[ -n $qold ]] && ! "$ENGINE" pull -q "$qold" >/dev/null 2>&1; then
        qold=""
    fi

    # One pass per argv prefix: flags are only comparable within the command
    # they are passed to, so each gets its own listing, control and verdicts.
    local prefix
    while IFS= read -r prefix; do
        replay_prefix "$prefix"
    done < <(printf '%s\n' "${contexts[@]}" | cut -f1 | sort -u)
    echo
}

# Replay every flag belonging to one argv prefix. Reads contexts/qimage/qold and
# the labels from check_image's scope.
replay_prefix() {
    local prefix=$1
    local -a flags=()
    mapfile -t flags < <(printf '%s\n' "${contexts[@]}" |
        awk -F'\t' -v p="$prefix" '$1 == p {print $2}' | sort -u)
    [[ ${#flags[@]} -eq 0 ]] && return 0

    local label="entrypoint"
    [[ -n $prefix ]] && label="\`${prefix}\`"

    # Positive control: if the binary cannot print usage here, no per-flag
    # verdict from it is trustworthy, and calling them all accepted would be
    # worse than reporting nothing.
    local new_help old_help=""
    new_help=$(help_flags "$qimage" "$prefix")
    if [[ -z $new_help ]]; then
        echo "- **SKIPPED**: ${label} produced no usage output here, so its"
        echo "  ${#flags[@]} flag(s) were not replayed. See the job log."
        log "no usage output from ${image} under '${prefix:-default entrypoint}'"
        annotate warning "$image" "could not run ${label_new} to replay the flags passed to ${prefix:-its entrypoint}"
        advisory=$((advisory + 1))
        return 0
    fi

    # pflag's MarkDeprecated also sets Hidden and FlagUsages cannot render
    # hidden flags, so --help under-reports what the binary accepts - which is
    # why it is cross-referenced with the replay rather than trusted alone. An
    # empty listing means "could not read it", so the diff needs both sides.
    local gone="" added=""
    [[ -n $qold ]] && old_help=$(help_flags "$qold" "$prefix")
    if [[ -n $old_help ]]; then
        gone=$(not_in "$old_help" "$new_help" | tr '\n' ' ')
        added=$(not_in "$new_help" "$old_help" | tr '\n' ' ')
    elif [[ -n $qold ]]; then
        log "no usage listing for ${qold} under '${prefix:-default entrypoint}'; skipping the --help comparison"
    fi

    local flag verdict ok=0 n_rejected=0 n_deprecated=0 n_undocumented=0 n_unknown=0
    local -a findings=()
    for flag in "${flags[@]}"; do
        verdict=$(probe "$qimage" "$prefix" "$flag")
        case ${verdict%%|*} in
        rejected)
            findings+=("- **REJECTED** \`${flag}\` - ${label} does not know this flag, so the container will not start with the arguments we render")
            mapfile -t -O "${#findings[@]}" findings < <(flag_sites "$flag")
            n_rejected=$((n_rejected + 1))
            if [[ $REPLAY_BLOCKING == 1 ]]; then
                annotate error "$image" "${label_new} rejects ${flag} under ${prefix:-its entrypoint}$(flag_sites_inline "$flag")"
                blocking=$((blocking + 1))
            else
                annotate warning "$image" "${label_new} rejects ${flag} under ${prefix:-its entrypoint}$(flag_sites_inline "$flag")"
                advisory=$((advisory + 1))
            fi
            ;;
        deprecated)
            findings+=("- DEPRECATED \`${flag}\` - ${verdict#*|}")
            mapfile -t -O "${#findings[@]}" findings < <(flag_sites "$flag")
            n_deprecated=$((n_deprecated + 1))
            annotate warning "$image" "${verdict#*|}$(flag_sites_inline "$flag")"
            advisory=$((advisory + 1))
            ;;
        unknown)
            findings+=("- UNVERIFIED \`${flag}\` - the probe container failed before parsing arguments")
            n_unknown=$((n_unknown + 1))
            advisory=$((advisory + 1))
            ;;
        *)
            ok=$((ok + 1))
            # Accepted, but undocumented and with no deprecation notice - the
            # one drift the replay alone cannot see.
            if [[ " ${gone} " == *" ${flag} "* ]]; then
                findings+=("- UNDOCUMENTED \`${flag}\` - dropped from \`--help\` in ${label_new} but still accepted")
                mapfile -t -O "${#findings[@]}" findings < <(flag_sites "$flag")
                n_undocumented=$((n_undocumented + 1))
                annotate warning "$image" "${flag} is gone from --help in ${label_new} but still accepted; likely on its way out$(flag_sites_inline "$flag")"
                advisory=$((advisory + 1))
            fi
            ;;
        esac
    done

    local tally="${ok} ok"
    ((n_rejected)) && tally+=", ${n_rejected} rejected"
    ((n_deprecated)) && tally+=", ${n_deprecated} deprecated"
    ((n_undocumented)) && tally+=", ${n_undocumented} undocumented"
    ((n_unknown)) && tally+=", ${n_unknown} unverified"
    echo "- ${label}: replayed ${#flags[@]} flag(s): ${tally}"
    [[ ${#findings[@]} -gt 0 ]] && printf '%s\n' "${findings[@]}"
    [[ -n ${added// /} ]] && echo "- new in \`--help\` for ${label}: ${added}"
    return 0
}

# --------------------------------------------------------------------------
# Entry point
# --------------------------------------------------------------------------

main() {
    # Without these preflights a missing yq makes every query come back empty and
    # read as "no annotations", and a stopped daemon lets the tag checks pass
    # (manifest inspect needs no daemon) while every replay silently skips.
    "$YQ" --version >/dev/null 2>&1 ||
        die "yq is not usable (tried '${YQ}'); run 'make yq' or set YQ"
    "$ENGINE" info >/dev/null 2>&1 ||
        die "container engine '${ENGINE}' cannot run containers - is the daemon started? Set ENGINE=podman to use podman instead"

    local base head bumps
    base=$(image_locations "$BASE_REF")
    head=$(image_locations "$HEAD_REF")

    # A bump is a version present for a (file, repo) at head and not at base.
    # Within each group the versions on both sides cancel out first, then what is
    # left is paired. Counting occurrences instead misfires the moment one is
    # inserted or deleted: with [A, B], dropping A promotes an untouched B and
    # invents an "A -> B" bump.
    bumps=$(awk -F'\t' '
        NR == FNR { base[$1 FS $2] = base[$1 FS $2] $3 "\n"; next }
        { head[$1 FS $2] = head[$1 FS $2] $3 "\n" }
        END {
            for (k in head) {
                nh = split(head[k], ht, "\n")
                nb = split(base[k], bt, "\n")
                split("", at); split("", rm); split("", bs); split("", hs)
                for (i = 1; i <= nb; i++) if (bt[i] != "") bs[bt[i]] = 1
                for (i = 1; i <= nh; i++) if (ht[i] != "") hs[ht[i]] = 1
                na = 0; nr = 0
                for (i = 1; i <= nh; i++) if (ht[i] != "" && !(ht[i] in bs)) at[++na] = ht[i]
                for (i = 1; i <= nb; i++) if (bt[i] != "" && !(bt[i] in hs)) rm[++nr] = bt[i]
                split(k, key, FS)
                for (i = 1; i <= na; i++)
                    print key[2] FS at[i] FS (i <= nr ? rm[i] : "") FS key[1]
            }
        }
    ' <(echo "$base") <(echo "$head") | sort -u)

    # Collapse per-file rows into one per (repo, new, old), carrying the files so
    # the annotation check can be scoped to them. The old version goes last: it
    # is empty for a newly referenced image, and IFS collapses consecutive tabs,
    # so an empty middle field would shift the ones after it.
    bumps=$(awk -F'\t' '
        { k = $1 FS $2 FS $3; f[k] = (k in f ? f[k] " " $4 : $4) }
        END { for (k in f) { split(k, p, FS); print p[1] FS p[2] FS f[k] FS p[3] } }
    ' <<<"$bumps" | sort)

    if [[ -z $bumps ]]; then
        echo "No image tag changes between ${BASE_REF} and ${HEAD_REF}."
        return 0
    fi

    CHANGED_FILES=$(git diff --name-only "$BASE_REF" "$HEAD_REF" -- "${SCAN_PATHS[@]}" || true)

    echo "## Image CLI flag drift"
    echo
    echo "Comparing \`${BASE_REF}\` to \`${HEAD_REF}\`."
    echo

    local repo old_tag new_tag files
    while IFS=$'\t' read -r repo new_tag files old_tag; do
        [[ -z $repo ]] && continue
        check_image "$repo" "$old_tag" "$new_tag" "$files"
    done <<<"$bumps"

    if [[ $blocking -gt 0 ]]; then
        echo "**FAIL** - ${blocking} blocking, ${advisory} advisory."
    elif [[ $advisory -gt 0 ]]; then
        echo "**PASS with findings** - ${advisory} advisory finding(s), none blocking."
    else
        echo "**PASS** - no drift."
    fi
    echo
    if [[ $advisory -gt 0 ]]; then
        echo "Advisory findings do not fail this check: flags are read from the manifests"
        echo "rather than from what the controller finally renders, so a flag stripped by a"
        echo "version-gated migration can show up here. A REJECTED flag still means the"
        echo "container will not start with the arguments we render - migrate or drop it in"
        echo "the same PR as the bump."
    fi
    [[ $blocking -gt 0 ]] && return 1
    return 0
}

exec 3>&1
main | tee -a "${GITHUB_STEP_SUMMARY:-/dev/null}"
exit "${PIPESTATUS[0]}"
