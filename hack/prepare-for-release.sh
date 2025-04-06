#!/usr/bin/env bash
# Helper script to bump the KServe version
# Usage:
#   ./hack/prepare-for-release.sh <prior_version> <new_version>

set -eo pipefail

# make sure the directory is the root of the repository
if [ $0 != "hack/prepare-for-release.sh" ]; then
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
  sed -i "s/\bv${PRIOR_VERSION}\b/v${NEW_VERSION}/g" ${readmeFile}
  sed -i "s/Version-v${pversion}/Version-v${nversion}/g" ${readmeFile}
  # sanity check, when doing final release update to the next rc version it might skip the double dash
  sed -i "s/Version-v${NEW_VERSION}/Version-v${nversion}/g" ${readmeFile}
done

for yaml in `find charts \( -name "Chart.yaml" -o -name "values.yaml" \)`; do
  # do not interact over empty files
  if [ ! -s "yaml" ]; then
     echo -e "\033[32mUpdating ${yaml}...\033[0m"
     sed -i "s/\bv${PRIOR_VERSION}\b/v${NEW_VERSION}/g" ${yaml}
  fi
done

# Update hack/generate-install.sh
echo -e "\033[32mUpdating hack/generate-install.sh...\033[0m"
sed -i "/\"v${PRIOR_VERSION}\"/a \    \"v${NEW_VERSION}\"" hack/generate-install.sh

# Update hack/quick_install.sh
echo -e "\033[32mUpdating hack/quick_install.sh...\033[0m"
sed -i "s/KSERVE_VERSION=v${PRIOR_VERSION}/KSERVE_VERSION=v${NEW_VERSION}/g" hack/quick_install.sh


# update python/kserve version
echo -e "\033[32mUpdating python/kserve version...\033[0m"
## if rcX release, it has no dash, e.g. 0.14.0rc1
new_no_dash_version=$(echo ${NEW_VERSION} | sed 's/-//g')
prior_no_dash_version=$(echo ${PRIOR_VERSION} | sed 's/-//g')
echo -e "\033[32mNo dash version updated to ${new_no_dash_version} and prior: ${prior_no_dash_version}...\033[0m"

echo "${new_no_dash_version}" > python/VERSION

for file in $(find python \( -name 'pyproject.toml' -o -name 'poetry.lock' \)); do
  echo -e "\033[32mUpdating ${file}\033[0m"
  if [[ ${file} == *"poetry.lock" ]]; then
    # make sure the previous line is name = "kserve"
    # there is a chance that the version being update be the same than other dependencies
    sed -i "/name = \"kserve\"/{N;s/${prior_no_dash_version}/${new_no_dash_version}/}" "${file}"
  else
    sed -i "s/${prior_no_dash_version}/${new_no_dash_version}/g" "${file}"
  fi
done

# update docs version
for file in $(find docs \( -name 'pyproject.toml' -o -name 'poetry.lock' \)); do
  echo -e "\033[32mUpdating ${file}\033[0m"
  if [[ ${file} == *"poetry.lock" ]]; then
    # make sure the previous line is name = "kserve"
    # there is a chance that the version being update be the same than other dependencies
    sed -i "/name = \"kserve\"/{N;s/${prior_no_dash_version}/${new_no_dash_version}/}" "${file}"
  else
    sed -i "s/${prior_no_dash_version}/${new_no_dash_version}/g" "${file}"
  fi
done

