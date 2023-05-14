#!/bin/bash

cd ./python

packages=$(find . -maxdepth 1 -type d)

for folder in ${packages[@]}
do
    echo "moving into folder ${folder}"
    if [[ ${folder} == 'plugin' || ! -f "${folder}/pyproject.toml" ]]; then
        echo -e "\033[33mskipping folder ${folder}\033[0m"
        continue
    fi
    pushd "${folder}"
        poetry install
    popd

done