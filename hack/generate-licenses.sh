#!/bin/bash
echo "Do not run this script. Copy paste the commands and execute the manual steps in between." && exit 1

## Generate a Golang License
# See https://github.com/kubeflow/testing/blob/master/py/kubeflow/testing/go-license-tools/README.md
go list -m all | cut -d ' ' -f 1 > dep.txt
python ../testing/py/kubeflow/testing/go-license-tools/get_github_repo.py 
python ../testing/py/kubeflow/testing/go-license-tools/get_github_license_info.py --github-api-token-file ~/.github_api_token
# Manually lookup all lines that contain 'Other'
python ../testing/py/kubeflow/testing/go-license-tools/concatenate_license.py
mv license.txt third_party/library/license_go.txt

## Generate a Python License
# See https://github.com/kubeflow/testing/blob/master/py/kubeflow/testing/python-license-tools/README.md
pipenv install -e python/alibiexplainer python/kfserving python/pytorchserver python/sklearnserver python/xgbserver
python ../testing/py/kubeflow/testing/python-license-tools/pipfile_to_github_repo.py
# See https://github.com/kubeflow/testing/blob/master/py/kubeflow/testing/go-license-tools/README.md
python ../testing/py/kubeflow/testing/go-license-tools/get_github_license_info.py --github-api-token-file ~/.github_api_token
# Manually lookup and fix all lines that contain 'Other'
python ../testing/py/kubeflow/testing/go-license-tools/concatenate_license.py
mv license.txt third_party/library/license_py.txt

## Concatenate the Licenses
rm third_party/library/license.txt # if exists
cat third_party/library/*.txt >> license.txt

## Cleanup
rm repo.txt license_info.csv Pipfile Pipfile.lock additional_license_info.csv dep.txt