#!/bin/bash

rm .github/dependabot.yml
cp .github/base_dependabot.yml.tmp .github/dependabot.yml

for directory in $(dirname $(find . -type f -name "*ockerfile*") | sort -u | cut -c2-); do
    yq eval -i ".updates += {\"package-ecosystem\":\"docker\",\"directory\":\"${directory}\",\"schedule\":{\"interval\":\"daily\"},\"open-pull-requests-limit\":10}" .github/dependabot.yml
done

for directory in $(dirname $(find . -type f -name "*requirements.txt") | sort -u | cut -c2-); do
     yq eval -i ".updates += {\"package-ecosystem\":\"pip\",\"directory\":\"${directory}\",\"schedule\":{\"interval\":\"daily\"},\"open-pull-requests-limit\":10}" .github/dependabot.yml
done

for directory in $(dirname $(find . -type f -name "go.*") | sort -u | cut -c2-); do
     yq eval -i ".updates += {\"package-ecosystem\":\"gomod\",\"directory\":\"${directory}\",\"schedule\":{\"interval\":\"daily\"},\"open-pull-requests-limit\":10}" .github/dependabot.yml
done
