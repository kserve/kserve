#!/bin/bash

# Copyright 2022 The KServe Authors.
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


echo "Creating etcd dependencies"
cat <<-EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: etcd
spec:
  ports:
    - name: etcd-client-port
      port: 2379
      protocol: TCP
      targetPort: 2379
  selector:
    app: etcd
EOF

echo "Creating etcd service"
cat <<-EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: etcd
  name: etcd
spec:
  replicas: 1
  selector:
    matchLabels:
      app: etcd
  template:
    metadata:
      labels:
        app: etcd
    spec:
      containers:
        - command:
            - etcd
            - --data-dir
            - /tmp/etcd.data
            - --listen-client-urls
            - http://0.0.0.0:2379
            - --advertise-client-urls
            - http://0.0.0.0:2379
          image: quay.io/coreos/etcd:v3.5.4
          name: etcd
          ports:
            - containerPort: 2379
              name: client
              protocol: TCP
            - containerPort: 2380
              name: server
              protocol: TCP
EOF

cat <<-EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: model-serving-etcd
stringData:
  etcd_connection: |
    {
      "endpoints": "http://etcd.default:2379",
      "root_prefix": "modelmesh-serving"
    }
EOF

echo "Creating minio dependencies"
cat <<-EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: minio
spec:
  ports:
    - name: minio-client-port
      port: 9000
      protocol: TCP
      targetPort: 9000
  selector:
    app: minio
EOF

cat <<-EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: minio
  name: minio
spec:
  replicas: 1
  selector:
    matchLabels:
      app: minio
  template:
    metadata:
      labels:
        app: minio
    spec:
      containers:
        - args:
            - server
            # - /data
            - /data1
          env:
            - name: MINIO_ACCESS_KEY
              value: AKIAIOSFODNN7EXAMPLE
            - name: MINIO_SECRET_KEY
              value: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
          # image: quay.io/cloudservices/minio:latest
          image: kserve/modelmesh-minio-examples:latest
          name: minio
EOF

cat <<-EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: storage-config
stringData:
  localMinIO: |
    {
      "type": "s3",
      "access_key_id": "AKIAIOSFODNN7EXAMPLE",
      "secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
      "endpoint_url": "http://minio:9000",
      "default_bucket": "modelmesh-example-models",
      "region": "us-south"
    }
EOF
