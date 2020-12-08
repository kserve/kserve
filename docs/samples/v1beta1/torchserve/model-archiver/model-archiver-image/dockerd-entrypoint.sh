#!/bin/bash
########)#############################################################
#### Script for model archiving and config.properties generation #####
######################################################################
set -e

BASE_PATH='/home/model-server'
MODEL_STORE=$BASE_PATH/model-store
CONFIG_PATH=$BASE_PATH/config

touch $CONFIG_PATH/config.properties

cat <<EOF > "$CONFIG_PATH"/config.properties
inference_address=http://0.0.0.0:8080
management_address=http://0.0.0.0:8081
number_of_netty_threads=4
job_queue_size=100
model_store="$MODEL_STORE"
model_snapshot=
EOF
truncate -s -1 "$CONFIG_PATH"/config.properties

CONFIG_PROPERTIES=$CONFIG_PATH/config.properties
PROPERTIES_JSON=$MODEL_STORE/properties.json

count=$(jq -c '. | length' "$PROPERTIES_JSON")
echo "{\"name\":\"startup.cfg\",\"modelCount\":\"3\",\"models\":{}}" | jq -c --arg count "${count}" '.["modelCount"]=$count' >> $CONFIG_PROPERTIES
sed -i 's/{}}//' $CONFIG_PROPERTIES
truncate -s -1 $CONFIG_PROPERTIES
# shellcheck disable=SC1091
jq -c '.[]' "$PROPERTIES_JSON" | while read -r i; do
    modelName=$(echo "$i" | jq -r '."model-name"')
    modelFile=$(echo "$i" | jq -r '."model-file"')
    version=$(echo "$i" | jq -r '."version"')
    serializedFile=$(echo "$i" | jq -r '."serialized-file"')
    extraFiles=$(echo "$i" | jq -r '."extra-files"')
    handler=$(echo "$i" | jq -r '."handler"')
    minWorkers=$(echo "$i" | jq -r '."min-workers"')
    maxWorkers=$(echo "$i" | jq -r '."max-workers"')
    batchSize=$(echo "$i" | jq -r '."batch-size"')
    maxBatchDelay=$(echo "$i" | jq -r '."max-batch-delay"')
    responseTimeout=$(echo "$i" | jq -r '."response-timeout"')
    marName=${modelName}.mar
    requirements=$(echo "$i" | jq -r '."requirements"')
    updatedExtraFiles=$(echo "$extraFiles" | tr "," "\n" | awk -v modelName="$MODEL_STORE" -v modelFile="$modelName" '{ print modelName"/"modelFile"/"$1 }' | paste -sd "," -)
    ########)#############################
    #### Support for custom handlers #####
    ######################################
    pyfile="$( cut -d '.' -f 2 <<< "$handler" )"
    if [ "$pyfile" == "py" ];
    then
        handler="$MODEL_STORE/$modelName/$handler"
    fi
    if [ -z "$modelFile" ]; then
        if [ -z "$extraFiles" ]; then
            torch-model-archiver --model-name "$modelName" --version "$version" --serialized-file "$MODEL_STORE/$modelName/$serializedFile" --export-path "$MODEL_STORE" --handler "$handler" -r "$requirements" --force
        else
            torch-model-archiver --model-name "$modelName" --version "$version" --serialized-file "$MODEL_STORE/$modelName/$serializedFile" --export-path "$MODEL_STORE" --extra-files "$updatedExtraFiles" --handler "$handler" -r "$requirements" --force
        fi
    else
        if [ -z "$extraFiles" ]; then
            torch-model-archiver --model-name "$modelName" --version "$version" --model-file "$MODEL_STORE/$modelName/$modelFile" --serialized-file "$MODEL_STORE/$modelName/$serializedFile" --export-path "$MODEL_STORE" --handler "$handler" -r "$requirements" --force
        else
            torch-model-archiver --model-name "$modelName" --version "$version" --model-file "$MODEL_STORE/$modelName/$modelFile" --serialized-file "$MODEL_STORE/$modelName/$serializedFile" --export-path "$MODEL_STORE" --extra-files "$updatedExtraFiles" --handler "$handler" -r "$requirements" --force
        fi
    fi
    echo "{\"modelName\":{\"version\":{\"defaultVersion\":true,\"marName\":\"sample.mar\",\"minWorkers\":\"sampleminWorkers\",\"maxWorkers\":\"samplemaxWorkers\",\"batchSize\":\"samplebatchSize\",\"maxBatchDelay\":\"samplemaxBatchDelay\",\"responseTimeout\":\"sampleresponseTimeout\"}}}" | 
    jq -c --arg modelName "$modelName" --arg version "$version" --arg marName "$marName" --arg minWorkers "$minWorkers" --arg maxWorkers "$maxWorkers" --arg batchSize "$batchSize" --arg maxBatchDelay "$maxBatchDelay" --arg responseTimeout "$responseTimeout" '.[$modelName]=."modelName" | .[$modelName][$version]=.[$modelName]."version" | .[$modelName][$version]."marName"=$marName | .[$modelName][$version]."minWorkers"=$minWorkers | .[$modelName][$version]."maxWorkers"=$maxWorkers | .[$modelName][$version]."batchSize"=$batchSize | .[$modelName][$version]."maxBatchDelay"=$maxBatchDelay | .[$modelName][$version]."responseTimeout"=$responseTimeout | del(."modelName", .[$modelName]."version")'  >> $CONFIG_PROPERTIES
    truncate -s -1 $CONFIG_PROPERTIES
done
sed -i 's/}{/,/g' $CONFIG_PROPERTIES
sed -i 's/}}}/}}}}/g' $CONFIG_PROPERTIES

# prevent docker exit
tail -f /dev/null


