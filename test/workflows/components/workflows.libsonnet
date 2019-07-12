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
      local testWorkerImage = "gcr.io/kubeflow-ci/test-worker";
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
                    name: "unit-test",
                    template: "unit-test",
                  },
                  {
                    name: "pylint-checking",
                    template: "pylint-checking",
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
                    name: "build-model-initializer",
                    template: "build-model-initializer",
                  },
                  {
                    name: "build-xgbserver",
                    template: "build-xgbserver",
                  },
                  {
                    name: "build-kfserving-executor",
                    template: "build-kfserving-executor",
                  },
                  {
                    name: "build-pytorchserver",
                    template: "build-pytorchserver",
                  },
                  {
                    name: "build-sklearnserver",
                    template: "build-sklearnserver",
                  },
                ],
                [
                  {
                    name: "run-tests",
                    template: "run-tests",
                  },
                ],
              ],
            },
            {
              name: "exit-handler",
              steps: [
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
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("run-tests", helmImage, [
              "test/scripts/run-tests.sh",
            ]),  // run tests
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-kfserving", testWorkerImage, [
              "test/scripts/build-kfserving.sh",
            ]),  // build-kfserving
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-alibi-explainer", testWorkerImage, [
              "test/scripts/build-python-image.sh", "alibiexplainer.Dockerfile", "alibi-explainer",
            ]),  // build-alibi-explainer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-model-initializer", testWorkerImage, [
              "test/scripts/build-python-image.sh", "model-initializer.Dockerfile", "model-initializer",
            ]),  // build-model-initializer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-xgbserver", testWorkerImage, [
              "test/scripts/build-python-image.sh", "xgb.Dockerfile", "xgbserver",
            ]),  // build-xgbserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-kfserving-executor", testWorkerImage, [
              "test/scripts/build-kfserving-executor.sh",
            ]),  // build-kfserving-executor
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-pytorchserver", testWorkerImage, [
              "test/scripts/build-python-image.sh", "pytorch.Dockerfile", "pytorchserver",
            ]),  // build-pytorchserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-sklearnserver", testWorkerImage, [
              "test/scripts/build-python-image.sh", "sklearn.Dockerfile", "sklearnserver",
            ]),  // build-sklearnserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("unit-test", testWorkerImage, [
              "test/scripts/unit-test.sh",
            ]),  // unit test
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
