#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
CONFIGS=$(find $DIR/../release | grep yaml)
PROJECT=kfserving

for i in $(gcloud alpha builds triggers list --project kfserving | grep ^id | cut -d ':' -f 2); do gcloud alpha builds triggers delete $i --project $PROJECT --quiet; done

for CONFIG in $CONFIGS; do
    gcloud alpha builds triggers create github --project $PROJECT --trigger-config $CONFIG
done