{
  listToMap:: function(v)
    {
      name: v[0],
      value: v[1],
    },

  parseEnv:: function(v)
    local pieces = std.split(v, ",");
    if v != "" && std.length(pieces) > 0 then
      std.map(
        function(i) $.listToMap(std.split(i, "=")),
        std.split(v, ",")
      )
    else [],


  // default parameters.
  defaultParams:: {
    project:: "kubeflow-ci",
    zone:: "us-east1-d",
    // Default registry to use.
    //registry:: "gcr.io/" + $.defaultParams.project,

    // The image tag to use.
    // Defaults to a value based on the name.
    versionTag:: null,

    // The name of the secret containing GCP credentials.
    gcpCredentialsSecretName:: "kubeflow-testing-credentials",
  },

  parts(namespace, name, overrides):: {
    // Workflow to run the e2e test.
    e2e(prow_env, bucket):
      local params = $.defaultParams + overrides;

      // mountPath is the directory where the volume to store the test data
      // should be mounted.
      local mountPath = "/mnt/" + "test-data-volume";
      // testDir is the root directory for all data for a particular test run.
      local testDir = mountPath + "/" + name;
      // outputDir is the directory to sync to GCS to contain the output for this job.
      local outputDir = testDir + "/output";
      local artifactsDir = outputDir + "/artifacts";
      local goDir = testDir + "/go";
      // Source directory where all repos should be checked out
      local srcRootDir = testDir + "/src";
      // The directory containing the kubeflow/kfserving repo
      local srcDir = srcRootDir + "/kubeflow/kfserving";
      local pylintSrcDir = srcDir + "/python";
      local testWorkerImage = "gcr.io/kubeflow-ci/test-worker-py3@sha256:b679ce5d7edbcc373fd7d28c57454f4f22ae987f200f601252b6dcca1fd8823b";
      local golangImage = "golang:1.9.4-stretch";
      // TODO(jose5918) Build our own helm image
      local pythonImage = "python:3.6-jessie";
      local helmImage = "volumecontroller/golang:1.9.2";
      // The name of the NFS volume claim to use for test files.
      // local nfsVolumeClaim = "kubeflow-testing";
      local nfsVolumeClaim = "nfs-external";
      // The name to use for the volume to use to contain test data.
      local dataVolume = "kubeflow-test-volume";
      local versionTag = if params.versionTag != null then
        params.versionTag
        else name;

      // The namespace on the cluster we spin up to deploy into.
      local deployNamespace = "kubeflow";
      // The directory within the kubeflow_testing submodule containing
      // py scripts to use.
      local k8sPy = srcDir;
      local kubeflowPy = srcRootDir + "/kubeflow/testing/py";

      local project = params.project;
      // GKE cluster to use
      // We need to truncate the cluster to no more than 40 characters because
      // cluster names can be a max of 40 characters.
      // We expect the suffix of the cluster name to be unique salt.
      // We prepend a z because cluster name must start with an alphanumeric character
      // and if we cut the prefix we might end up starting with "-" or other invalid
      // character for first character.
      local cluster =
        if std.length(name) > 40 then
          "z" + std.substr(name, std.length(name) - 39, 39)
        else
          name;
      local zone = params.zone;
      local registry = params.registry;
      {
        // Build an Argo template to execute a particular command.
        // step_name: Name for the template
        // command: List to pass as the container command.
        buildTemplate(step_name, image, command):: {
          name: step_name,
          retryStrategy: {
            limit: 3,
            retryPolicy: "Always",
            backoff: {
              duration: 1,
              factor: 2,
              maxDuration: "1m",
            },
          },
          container: {
            command: command,
            image: image,
            workingDir: srcDir,
            env: [
              {
                // Add the source directories to the python path.
                name: "PYTHONPATH",
                value: k8sPy + ":" + kubeflowPy,
              },
              {
                // Set the GOPATH
                name: "GOPATH",
                value: goDir,
              },
              {
                name: "CLUSTER_NAME",
                value: cluster,
              },
              {
                name: "GCP_ZONE",
                value: zone,
              },
              {
                name: "GCP_PROJECT",
                value: project,
              },
              {
                name: "GCP_REGISTRY",
                value: registry,
              },
              {
                name: "DEPLOY_NAMESPACE",
                value: deployNamespace,
              },
              {
                name: "GOOGLE_APPLICATION_CREDENTIALS",
                value: "/secret/gcp-credentials/key.json",
              },
              {
                name: "GIT_TOKEN",
                valueFrom: {
                  secretKeyRef: {
                    name: "github-token",
                    key: "github_token",
                  },
                },
              },
            ] + prow_env,
            volumeMounts: [
              {
                name: dataVolume,
                mountPath: mountPath,
              },
              {
                name: "github-token",
                mountPath: "/secret/github-token",
              },
              {
                name: "gcp-credentials",
                mountPath: "/secret/gcp-credentials",
              },
            ],
          },
        },  // buildTemplate

        apiVersion: "argoproj.io/v1alpha1",
        kind: "Workflow",
        metadata: {
          name: name,
          namespace: namespace,
        },
        spec: {
          entrypoint: "e2e",
          volumes: [
            {
              name: "github-token",
              secret: {
                secretName: "github-token",
              },
            },
            {
              name: "gcp-credentials",
              secret: {
                secretName: params.gcpCredentialsSecretName,
              },
            },
            {
              name: dataVolume,
              persistentVolumeClaim: {
                claimName: nfsVolumeClaim,
              },
            },
          ],  // volumes
          // onExit specifies the template that should always run when the workflow completes.
          onExit: "exit-handler",
          templates: [
            {
              name: "e2e",
              steps: [
                [{
                  name: "checkout",
                  template: "checkout",
                }],
                [
                  {
                    name: "sdk-test",
                    template: "sdk-test",
                  },
                  {
                    name: "pylint-checking",
                    template: "pylint-checking",
                  },
                ],
                [
                  {
                    name: "setup-cluster",
                    template: "setup-cluster",
                  },
                ],
                [
                  {
                    name: "build-kfserving-manager",
                    template: "build-kfserving",
                  },
                  {
                    name: "build-alibi-explainer",
                    template: "build-alibi-explainer",
                  },
                  {
                    name: "build-aix-explainer",
                    template: "build-aix-explainer",
                  },
                  {
                    name: "build-storage-initializer",
                    template: "build-storage-initializer",
                  },
                  {
                    name: "build-xgbserver",
                    template: "build-xgbserver",
                  },
                  {
                    name: "build-logger",
                    template: "build-logger",
                  },
                  {
                    name: "build-batcher",
                    template: "build-batcher",
                  },
                  {
                    name: "build-agent",
                    template: "build-agent",
                  },
                  {
                    name: "build-custom-image-transformer",
                    template: "build-custom-image-transformer",
                  },
                  {
                    name: "build-custom-bert-transformer",
                    template: "build-custom-bert-transformer",
                  },
                  {
                    name: "build-pytorchserver",
                    template: "build-pytorchserver",
                  },
                  {
                    name: "build-pytorchserver-gpu",
                    template: "build-pytorchserver-gpu",
                  },
                  {
                    name: "build-sklearnserver",
                    template: "build-sklearnserver",
                  },
                ],
                [
                  {
                    name: "run-e2e-tests",
                    template: "run-e2e-tests",
                  },
                ],
              ],
            },
            {
              name: "exit-handler",
              steps: [
                [{
                  name: "teardown-cluster",
                  template: "teardown-cluster",

                }],
                [{
                  name: "copy-artifacts",
                  template: "copy-artifacts",
                }],
              ],
            },
            {
              name: "checkout",
              container: {
                command: [
                  "/usr/local/bin/checkout.sh",
                  srcRootDir,
                ],
                env: prow_env + [{
                  name: "EXTRA_REPOS",
                  value: "kubeflow/testing@HEAD",
                }],
                image: testWorkerImage,
                volumeMounts: [
                  {
                    name: dataVolume,
                    mountPath: mountPath,
                  },
                ],
              },
            },  // checkout
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("setup-cluster",testWorkerImage, [
              "test/scripts/create-cluster.sh",
            ]),  // setup cluster
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("run-e2e-tests",testWorkerImage, [
              "test/scripts/run-e2e-tests.sh",
            ]),  // deploy kfserving
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("teardown-cluster",testWorkerImage, [
              "test/scripts/delete-cluster.sh",
             ]),  // teardown cluster
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-kfserving", testWorkerImage, [
              "test/scripts/build-kfserving.sh",
            ]),  // build-kfserving
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-alibi-explainer", testWorkerImage, [
              "test/scripts/build-python-image.sh", "alibiexplainer.Dockerfile", "alibi-explainer", "latest"
            ]),  // build-alibi-explainer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-aix-explainer", testWorkerImage, [
              "test/scripts/build-python-image.sh", "aixexplainer.Dockerfile ", "aix-explainer", "latest"
            ]),  // build-aix-explainer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-storage-initializer", testWorkerImage, [
              "test/scripts/build-python-image.sh", "storage-initializer.Dockerfile", "storage-initializer", "latest"
            ]),  // build-storage-initializer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-xgbserver", testWorkerImage, [
              "test/scripts/build-python-image.sh", "xgb.Dockerfile", "xgbserver", "latest"
            ]),  // build-xgbserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-logger", testWorkerImage, [
              "test/scripts/build-logger.sh",
            ]),  // build-logger
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-batcher", testWorkerImage, [
              "test/scripts/build-batcher.sh",
            ]),  // build-batcher
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-custom-image-transformer", testWorkerImage, [
              "test/scripts/build-custom-image-transformer.sh",
            ]),  // build-custom-image-transformer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-custom-bert-transformer", testWorkerImage, [
              "test/scripts/build-custom-bert-transformer.sh",
            ]),  // build-custom-bert-transformer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-pytorchserver", testWorkerImage, [
              "test/scripts/build-python-image.sh", "pytorch.Dockerfile", "pytorchserver", "latest"
            ]),  // build-pytorchserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-pytorchserver-gpu", testWorkerImage, [
              "test/scripts/build-python-image.sh", "pytorch-gpu.Dockerfile", "pytorchserver", "latest-gpu"
            ]),  // build-pytorchserver-gpu
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-sklearnserver", testWorkerImage, [
              "test/scripts/build-python-image.sh", "sklearn.Dockerfile", "sklearnserver", "latest"
            ]),  // build-sklearnserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("unit-test", testWorkerImage, [
              "test/scripts/unit-test.sh",
            ]),  // unit test
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("sdk-test", testWorkerImage, [
              "test/scripts/sdk-test.sh",
            ]),  // sdk unit test
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("pylint-checking", testWorkerImage, [
              "python",
              "-m",
              "kubeflow.testing.test_py_lint",
              "--artifacts_dir=" + artifactsDir,
              "--src_dir=" + pylintSrcDir,
            ]),  // pylint-checking
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("copy-artifacts", testWorkerImage, [
              "python",
              "-m",
              "kubeflow.testing.prow_artifacts",
              "--artifacts_dir=" + outputDir,
              "copy_artifacts",
              "--bucket=" + bucket,
            ]),  // copy-artifacts
          ],  // templates
        },
      },  // e2e
  },  // parts
}
