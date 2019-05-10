{
  global: {
    // User-defined global parameters; accessible to all component and environments, Ex:
    // replicas: 4,
  },
  components: {
    "workflows": {
      bucket: "kubeflow-ci_temp",
      name: "some-very-very-very-very-very-long-name-kfserving-presubmit-test-74-977c",
      namespace: "kubeflow-test-infra",
      prow_env: "JOB_NAME=kfserving-presubmit-test,JOB_TYPE=presubmit,PULL_NUMBER=74,REPO_NAME=kfserving,REPO_OWNER=kubeflow,BUILD_NUMBER=977c",
      versionTag: null,
    },
  },
}
