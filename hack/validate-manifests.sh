#!/bin/bash

set -euo pipefail

bold='\033[1m'
normal='\033[0m'
underline='\033[4m'

validate_kustomization() {
    local kustomization_file="$1"
    local project_root="${2:-$(git rev-parse --show-toplevel)}"

    local dir
    dir=$(dirname "$kustomization_file")
    local relative_path=${kustomization_file#"$project_root/"}
    local message="${bold}Validating${normal} ${underline}$relative_path${normal}"

    echo -n -e "⏳ ${message}"
    if output=$(kustomize build --load-restrictor LoadRestrictionsNone --stack-trace "$dir" 2>&1); then
        echo -e "\r✅ ${message}"
        return 0
    else
        echo -e "\r❌ ${message}"
        echo "$output"
        return 1
    fi
}

validate_all() {
    local project_root="$1"; shift
    local ignore_paths=("$@")
    local exit_code=0

    while IFS= read -r -d '' kustomization_file; do
        local skip=false
        for ignore in "${ignore_paths[@]}"; do
            if [[ "$kustomization_file" == "$project_root/$ignore"* ]]; then
                skip=true
                break
            fi
        done

        if [[ $skip == true ]]; then
            continue
        fi

        if ! validate_kustomization "$kustomization_file" "$project_root"; then
            exit_code=1
        fi
    done < <(find "$project_root" -name "kustomization.yaml" -type f -print0)

    return $exit_code
}

# When script is not sourced, but directly invoked
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)

    # Ensure the kernelcache webhook signing key exists before running kustomize build.
    # config/secret/mutation.env is not committed to git; generate a throwaway key if missing.
    _mutation_env="${PROJECT_ROOT}/config/secret/mutation.env"
    mkdir -p "$(dirname "${_mutation_env}")"
    if [[ ! -s "${_mutation_env}" ]]; then
        printf 'MUTATION_SIGNING_KEY=%s\n' "$(head -c 32 /dev/urandom | base64 | tr -d '\n')" > "${_mutation_env}"
    fi
    unset _mutation_env

    ignore_paths=()
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --ignore)
                shift
                [[ $# -gt 0 ]] || { echo "Error: --ignore requires a path"; exit 1; }
                ignore_paths+=("$1")
                ;;
            *)
                echo "Unknown argument: $1"
                exit 1
                ;;
        esac
        shift
    done

    validate_all "$PROJECT_ROOT" "${ignore_paths[@]}"
    exit $?
fi
