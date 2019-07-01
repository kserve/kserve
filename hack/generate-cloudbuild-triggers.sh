#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
CONFIGS=$(find $DIR/../release | grep yaml)

# Not currently idempotent! Use with care. Will be resolve in July 2019
for CONFIG in $CONFIGS; do
    gcloud alpha builds triggers create github --project kfserving --trigger_config $CONFIG
done

