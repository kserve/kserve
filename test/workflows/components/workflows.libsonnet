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

    // The image tag to use.
    // Defaults to a value based on the name.
    versionTag:: null,
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
      // The directory containing the kserve/kserve repo
      local srcDir = srcRootDir + "/kserve/kserve";
      local pylintSrcDir = srcDir + "/python";
      local kanikoExecutorImage = "gcr.io/kaniko-project/executor:v1.0.0";
      //local testWorkerImage = "public.ecr.aws/j1r0q0g6/kubeflow-testing:latest";
      //use kserve testing-worker image for go 1.17
      local testWorkerImage = "kserve/testing-worker:latest";
      local golangImage = "golang:1.17-stretch";
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
      local kservePy = srcDir  + "/python/kserve";

      local project = params.project;
      // GKE cluster to use
      // We need to truncate the cluster to no more than 40 characters because
      // cluster names can be a max of 40 characters.
      // We expect the suffix of the cluster name to be unique salt.
      // We prepend a z because cluster name must start with an alphanumeric character
      // and if we cut the prefix we might end up starting with "-" or other invalid
      // character for first character.
      local cluster =
        if std.length(name) > 80 then
          "z" + std.substr(name, std.length(name) - 79, 79)
        else
          name;
      local zone = params.zone;
      local registry = params.registry;
      {
        // Build an Argo template to execute a particular command.
        // step_name: Name for the template
        // command: List to pass as the container command.
        buildTemplate(step_name, image, command, env_vars=[], volume_mounts=[]):: {
          name: step_name,
          retryStrategy: {
            limit: "3",
            retryPolicy: "Always",
            backoff: {
              duration: "1",
              factor: "2",
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
                value: k8sPy + ":" + kubeflowPy + ":" + kservePy,
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
                name: "GCP_REGISTRY",
                value: registry,
              },
              {
                name: "DEPLOY_NAMESPACE",
                value: deployNamespace,
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
              {
                name: "AWS_REGION",
                value: "us-west-2",
              },
              {
                name: "AWS_ACCESS_KEY_ID",
                valueFrom: {
                  secretKeyRef: {
                    name: "aws-credentials",
                    key: "AWS_ACCESS_KEY_ID",
                  },
                },
              },
              {
                name: "AWS_SECRET_ACCESS_KEY",
                valueFrom: {
                  secretKeyRef: {
                    name: "aws-credentials",
                    key: "AWS_SECRET_ACCESS_KEY",
                  },
                },
              },
            ] + prow_env + env_vars,
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
                name: "aws-secret",
                mountPath: "/root/.aws/",
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
              name: dataVolume,
              persistentVolumeClaim: {
                claimName: nfsVolumeClaim,
              },
            },
            {
              name: "docker-config",
              configMap: {
                name: "docker-config",
              },
            },
            {
              name: "aws-secret",
              secret: {
                secretName: "aws-secret",
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
                    name: "pylint-checking",
                    template: "pylint-checking",
                  },
                ],
                [
                  {
                    name: "build-kserve-manager",
                    template: "build-kserve",
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
                    name: "build-art-explainer",
                    template: "build-art-explainer",
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
                    name: "build-agent",
                    template: "build-agent",
                  },
                  {
                    name: "build-custom-image-transformer",
                    template: "build-custom-image-transformer",
                  },
                  {
                    name: "build-paddleserver",
                    template: "build-paddleserver",
                  },
                  {
                    name: "build-sklearnserver",
                    template: "build-sklearnserver",
                  },
                  {
                    name: "build-pmmlserver",
                    template: "build-pmmlserver",
                  },
                  {
                    name: "build-lgbserver",
                    template: "build-lgbserver",
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
                  name: "e2e-tests-post-process",
                  template: "e2e-tests-post-process",
                }],
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
            ]),  // deploy kserve
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("teardown-cluster",testWorkerImage, [
              "test/scripts/delete-cluster.sh",
             ]),  // teardown cluster
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("e2e-tests-post-process",testWorkerImage, [
              "test/scripts/post-e2e-tests.sh",
             ]),  // run debug and clean up steps after running e2e test
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-kserve", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/Dockerfile",
              "--context=dir://" + srcDir,
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/kserve-controller:$(PULL_BASE_SHA)",
            ]),  // build-kserve
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-alibi-explainer", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/python/alibiexplainer.Dockerfile",
              "--context=dir://" + srcDir + "/python",
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/alibi-explainer:$(PULL_BASE_SHA)",
            ]),  // build-alibi-explainer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-aix-explainer", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/python/aixexplainer.Dockerfile",
              "--context=dir://" + srcDir + "/python",
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/aix-explainer:$(PULL_BASE_SHA)",
            ]),  // build-aix-explainer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-art-explainer", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/python/artexplainer.Dockerfile",
              "--context=dir://" + srcDir + "/python",
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/art-explainer:$(PULL_BASE_SHA)",
            ]),  // build-art-explainer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-storage-initializer", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/python/storage-initializer.Dockerfile",
              "--context=dir://" + srcDir + "/python",
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/storage-initializer:$(PULL_BASE_SHA)",
            ]),  // build-storage-initializer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-xgbserver", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/python/xgb.Dockerfile",
              "--context=dir://" + srcDir + "/python",
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/xgbserver:$(PULL_BASE_SHA)",
            ]),  // build-xgbserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-agent", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/agent.Dockerfile",
              "--context=dir://" + srcDir,
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/agent:$(PULL_BASE_SHA)",
            ]),  // build-agent
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-custom-image-transformer", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/python/custom_transformer.Dockerfile",
              "--context=dir://" + srcDir + "/python",
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/image-transformer:$(PULL_BASE_SHA)",
            ]),  // build-custom-image-transformer
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-pytorchserver", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/python/pytorch.Dockerfile",
              "--context=dir://" + srcDir + "/python",
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/pytorchserver:$(PULL_BASE_SHA)",
            ]),  // build-pytorchserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-pytorchserver-gpu", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/python/pytorch-gpu.Dockerfile",
              "--context=dir://" + srcDir + "/python",
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/pytorchserver:$(PULL_BASE_SHA)-gpu",
            ]),  // build-pytorchserver-gpu
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-paddleserver", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/python/paddle.Dockerfile",
              "--context=dir://" + srcDir + "/python",
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/paddleserver:$(PULL_BASE_SHA)",
            ]),  // build-paddleserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-sklearnserver", kanikoExecutorImage, [
              "/kaniko/executor",
              "--dockerfile=" + srcDir + "/python/sklearn.Dockerfile",
              "--context=dir://" + srcDir + "/python",
              "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/sklearnserver:$(PULL_BASE_SHA)",
            ]),  // build-sklearnserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-pmmlserver", kanikoExecutorImage, [
                "/kaniko/executor",
                "--dockerfile=" + srcDir + "/python/pmml.Dockerfile",
                "--context=dir://" + srcDir + "/python",
                "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/pmmlserver:$(PULL_BASE_SHA)",
            ]),  // build-pmmlserver
            $.parts(namespace, name, overrides).e2e(prow_env, bucket).buildTemplate("build-lgbserver", kanikoExecutorImage, [
                "/kaniko/executor",
                "--dockerfile=" + srcDir + "/python/lgb.Dockerfile",
                "--context=dir://" + srcDir + "/python",
                "--destination=" + "809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/lgbserver:$(PULL_BASE_SHA)",
            ]),  // build-lgbserver
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
              "kubeflow.testing.cloudprovider.aws.prow_artifacts",
              "--artifacts_dir=" + outputDir,
              "copy_artifacts_to_s3",
              "--bucket=" + bucket,
            ]),  // copy-artifacts
          ],  // templates
        },
      },  // e2e
  },  // parts
}
