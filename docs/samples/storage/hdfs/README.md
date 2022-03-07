
# HDFS Storage

KServe HDFS support requires a HDFS cluster that supports [WebHDFS](https://hadoop.apache.org/docs/r1.0.4/webhdfs.html).
Kerberos authentication is also supported.

HDFS support uses the [hdfscli](https://hdfscli.readthedocs.io/en/latest/) python bindings.

### Create a Kubernetes Secret

You need to create a Kubernetes secret with details of how to connect to your HDFS cluster. Below is a list of required and
optional variables you can use in the secret.

```bash
# Example of creating a secret using kubectl
$ kubectl create secret generic hdfscreds \
    --from-literal=HDFS_NAMENODE="https://host1:port;https://host2:port" \
    --from-file=TLS_CERT=./client.crt \
    --from-file=TLS_KEY=./client.key \
    --from-literal=TLS_SKIP_VERIFY="true"
    --from-literal=HDFS_ROOTPATH="/user/myuser" \
    --from-literal=HEADERS='{"x-my-container": "my-container"}'
```

```yaml
# Example of creating a secret using yaml file
apiVersion: v1
kind: Secret
metadata:
  name: hdfscreds
type: Opaque
stringData:
  HDFS_NAMENODE: xxxx
  ...
```

#### Required Variables

- `HDFS_NAMENODE`: Hostname or IP address of HDFS namenode, prefixed with protocol, followed by WebHDFS port on namenode. You may also specify multiple URLs separated by semicolons for High Availability support e.g. `https://domain1:port;https://domain2:port`

#### Optional Variables

- `USER_PROXY`: The user to proxy as when connecting to WebHDFS
- `HDFS_ROOTPATH`: Root path, this will be prefixed to all HDFS paths passed to the client. If the root is relative, the path will be assumed relative to the user’s home directory. Default: `"/"`
- `TLS_CERT`: TLS certificate to use when connecting to WebHDFS
- `TLS_KEY`: Key part of your TLS certificate
- `TLS_CA`: The CA to use to verify your TLS certificate
- `TLS_SKIP_VERIFY`: Whether to skip verifying the TLS certificate. Default: `"false"`
- `HEADERS`: Headers to add to the WebHDFS requests. Given as a json string e.g. `'{"x-my-container": "my-container"}'`
- `N_THREADS`: Number of threads to use when downloading files from WebHDFS. Default: `"2"`

#### Kerberos Authentication

You can connect to Kerberized clusters by specifying both of the variables below

- `KERBEROS_KEYTAB`: Kerberos keytab file
- `KERBEROS_PRINCIPAL`: Kerberos principal for the kerberos keytab e.g. `account@REALM`

```bash
# Example of creating a secret using kubectl to connect to a Kerberized cluster
$ kubectl create secret generic hdfscreds \
    --from-literal=HDFS_NAMENODE="https://host1:port;https://host2:port" \
    --from-file=KERBEROS_KEYTAB=./account.keytab \
    --from-literal=KERBEROS_PRINCIPAL="account@REALM"
```


### Attach to Service Account

KServe will check for secrets attached to the service account used for the `InferenceService`. Create a service account and attach the above `hdfscreds` secret.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: sa
secrets:
- name: hdfscreds
```

You can then specify to use this new service account in the `InferenceService` YAML.

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: model-test
spec:
  predictor:
    serviceAccountName: sa
    model:
    	storageUri: "hdfs://path/to/model"
```

When the `InferenceService` is created, the controller will check all secrets attached to the given service
account and look for a secret containing the key `HDFS_NAMENODE`. If found, all key-values in that secret will
then be attached to the storage-initializer pod for downloading your model(s).

### Using the new storage spec

Create a secret called `storage-config` in the namespace that the `InferenceService` will run.

The json config keys are the same as the variables above for storageUri. The difference is that the values of variables which reference files should be encoded to base64 so that they can be safely
used in the json. This applies to: `TLS_CERT` `TLS_KEY` `TLS_CA` `KERBEROS_KEYTAB`

```YAML
apiVersion: v1
kind: Secret
metadata:
  name: storage-config
type: Opaque
stringData:
  internalhdfs: |
    {
      "type": "hdfs",
      "HDFS_NAMENODE": "https://domain1:port;https://domain2:port",
      "KERBEROS_PRINCIPAL": "myaccount@REALM",
      "KERBEROS_KEYTAB": "<base64-encoded-data>"
    }
```

Example of encoding a file to base64. You should then copy the b64 value and use it in the above json configuration.

```bash
$ cat kerberos.keytab | base64 > kerberos.keytab.b64
# Copy data to clipboard on osx using pbcopy
$ cat kerberos.keytab.b64 | pbcopy
```

Then create an `InferenceService` using the new storage spec

```YAML
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: my-model
spec:
  predictor:
    model:
      modelFormat:
        name: tensorflow
      storage:
        key: internalhdfs
        path: /user/myuser/path/to/model
```