#!/usr/bin/env bash
# Helper script to bump the KServe version
# Usage:
#   ./hack/release/prepare-for-release.sh <prior_version> <new_version>

set -eo pipefail

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  SED_INPLACE=(-i '')
else
  SED_INPLACE=(-i)
fi

# make sure the directory is the root of the repository
if [ $0 != "hack/release/prepare-for-release.sh" ]; then
  echo -e "\033[31mError: run the script from the repository's root directory\033[0m"
  exit 1
fi

# set prior and next version from parameters
if [ "$#" -ne 2 ]; then
  echo "Usage: $0 <prior_version> <new_version>"
  exit 1
fi

PRIOR_VERSION=$1
NEW_VERSION=$2

if [ "${PRIOR_VERSION}" == "${NEW_VERSION}" ]; then
  echo -e "\033[31mError: versions cannot be the same.\033[0m"
  exit 1
fi

# check if the new version is greater than the prior version
p=$(echo ${PRIOR_VERSION} | cut -d '-' -f 1)
n=$(echo ${NEW_VERSION} | cut -d '-' -f 1)
# not allow same version to be update to rc1, e.g. 0.14.0 to 0.14.0-rc1
if [[ ${PRIOR_VERSION} != *"-"* ]] &&  [[ ${NEW_VERSION} == *"-"* ]]; then
  if [[ ${PRIOR_VERSION} == ${n} ]]; then
    # not allow same version to be update to rc1, e.g. 0.14.0 to 0.14.0-rc1
    echo -e "\033[31mError: New version must be greater than the prior version.\033[0m"
    exit 1
  fi
fi
if [[ $(printf '%s\n' "${PRIOR_VERSION}" "${NEW_VERSION}" | sort -V | head -n1) != "${PRIOR_VERSION}" ]]; then
  # handle update from rc to final version, e.g. 0.14.0-rc1 to 0.14.0
  if [[ ${PRIOR_VERSION} == *"-"* ]] &&  [[ ${NEW_VERSION} != *"-"* ]]; then
    # Allow update from rc to final version
    :
  else
    echo -e "\033[31mError: New version must be greater than the prior version.\033[0m"
    exit 1
  fi
fi

# make a pattern to match the versions, example: 0.13.1 -> 0\.13\.1.rc1
# it will match d.dd.d-xxx or d.dd.d
VERSION_PATTERN="^[0-9]+\.[0-9]{2}\.[0-9]+(-[a-zA-Z0-9]{1,3})?$"

# check if the new version matches the pattern
if [[ ! ${NEW_VERSION} =~ $VERSION_PATTERN ]]; then
  echo -e "\033[31mError: New version does not match the required pattern.\033[0m"
  exit 1
fi

# Display a warning message in yellow
echo -e "\033[33mWarning: The version update will replace ${PRIOR_VERSION} to ${NEW_VERSION}. Press Enter to continue...\033[0m"
read

# the following steps will perform version updates based on the prior version

# At some places there is badge that has this pattern: Version-v0.14.0--rc1
# using double dashes "--". We need to make sure to handle this case.`
pversion=""
nversion=""

if [[ ${NEW_VERSION} == *"-"* ]]; then
  nversion=$(echo ${NEW_VERSION} | sed 's/-/--/g')
else
  nversion=${NEW_VERSION}
fi
if [[ ${PRIOR_VERSION} == *"-"* ]]; then
  pversion=$(echo ${PRIOR_VERSION} | sed 's/-/--/g')
else
  pversion=${PRIOR_VERSION}
fi
echo "Normalized versions for the charts badge: prior: $pversion - new: $nversion"

# Charts
echo -e "\033[32mUpdating charts...\033[0m"
for readmeFile in `find charts -name README.md`; do
  echo -e "\033[32mUpdating ${readmeFile}...\033[0m"
  # Update badge version first (before general version replacement)
  sed "${SED_INPLACE[@]}" \
    -e "s/Version-v${pversion}/Version-v${nversion}/g" \
    -e "s/\bv${PRIOR_VERSION}\b/v${NEW_VERSION}/g" \
    ${readmeFile}
done

for yaml in `find charts \( -name "Chart.yaml" -o -name "values.yaml" \)`; do
  # skip empty files
  if [ -s "${yaml}" ]; then
     echo -e "\033[32mUpdating ${yaml}...\033[0m"
     sed "${SED_INPLACE[@]}" "s/\bv${PRIOR_VERSION}\b/v${NEW_VERSION}/g" ${yaml}
  fi
done

# Add new version to RELEASES array(if not already present)
echo -e "\033[32mUpdating hack/release/generate-install.sh...\033[0m"
if grep -q "\"v${NEW_VERSION}\"" hack/release/generate-install.sh; then
  echo -e "\033[33mVersion v${NEW_VERSION} already exists in hack/release/generate-install.sh, skipping...\033[0m"
else
  sed "${SED_INPLACE[@]}" "/\"v${PRIOR_VERSION}\"/a \\
    \"v${NEW_VERSION}\"" hack/release/generate-install.sh
fi

# Update kserve-deps.env
echo -e "\033[32mUpdating kserve-deps.env...\033[0m"
sed "${SED_INPLACE[@]}" "s/KSERVE_VERSION=v${PRIOR_VERSION}/KSERVE_VERSION=v${NEW_VERSION}/g" kserve-deps.env
sed "${SED_INPLACE[@]}" "s/\bv${PRIOR_VERSION}\b/v${NEW_VERSION}/g" charts/_common/common-sections.yaml

# update python/kserve and docs versions
echo -e "\033[32mUpdating python/kserve and docs versions...\033[0m"
## if rcX release, it has no dash, e.g. 0.14.0rc1
new_no_dash_version=$(echo ${NEW_VERSION} | sed 's/-//g')
prior_no_dash_version=$(echo ${PRIOR_VERSION} | sed 's/-//g')
# Escape dots for use in sed regex
escaped_new_version=$(echo ${new_no_dash_version} | sed 's/\./\\./g')
escaped_prior_version=$(echo ${prior_no_dash_version} | sed 's/\./\\./g')
echo -e "\033[32mNo dash version updated to ${new_no_dash_version} and prior: ${prior_no_dash_version}...\033[0m"

echo "${new_no_dash_version}" > python/VERSION

for file in $(find python docs \( -name 'pyproject.toml' -o -name 'uv.lock' \)); do
  echo -e "\033[32mUpdating ${file}\033[0m"
  if [[ ${file} == *"uv.lock" ]]; then
    # make sure the previous line is name = "kserve"
    # there is a chance that the version being update be the same than other dependencies
    sed "${SED_INPLACE[@]}" "/name = \"kserve\"/{N;s|${prior_no_dash_version}|${new_no_dash_version}|;}" "${file}"
  else
    # Only update kserve/kserve-storage versions, not external package versions
    sed "${SED_INPLACE[@]}" \
      -e "s|version = \"${prior_no_dash_version}\"|version = \"${new_no_dash_version}\"|" \
      -e "s|kserve-storage==${prior_no_dash_version}|kserve-storage==${new_no_dash_version}|g" \
      "${file}"
  fi
done

# Update Python dependency lock files
echo -e "\033[32mUpdating Python dependency lock files (uv-lock)...\033[0m"
make uv-lock
if [ $? -ne 0 ]; then
  echo -e "\033[31mError: Failed to update uv.lock files\033[0m"
  exit 1
fi

# Run precommit checks
echo -e "\033[32mRunning precommit checks (lint, format, vet)...\033[0m"
make precommit
if [ $? -ne 0 ]; then
  echo -e "\033[31mError: Precommit checks failed. Please fix the issues and re-run.\033[0m"
  exit 1
fi

# Generate install manifests
echo -e "\033[32mGenerating install manifests...\033[0m"
./hack/release/generate-install.sh "v${NEW_VERSION}"
if [ $? -ne 0 ]; then
  echo -e "\033[31mError: Failed to generate install manifests\033[0m"
  exit 1
fi

echo -e "\033[32m✓ All release preparation steps completed successfully!\033[0m"
echo -e "\033[33mNext steps:\033[0m"
echo -e "  1. Review the changes: git status"
echo -e "  2. Commit the changes"
echo -e "  3. Push and create a PR"
