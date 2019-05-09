local params = import '../../components/params.libsonnet';

params + {
  components+: {
    // Insert component parameter overrides here. Ex:
    // guestbook +: {
    // name: "guestbook-dev",
    // replicas: params.global.replicas,
    // },
    workflows+: {
      bucket: 'kubeflow-releasing-artifacts',
      gcpCredentialsSecretName: 'gcp-credentials',
      name: 'jlewi-tf-operator-release-403-2f58',
      namespace: 'kubeflow-releasing',
      project: 'kubeflow-releasing',
      prow_env: 'JOB_NAME=tf-operator-release,JOB_TYPE=presubmit,PULL_NUMBER=403,REPO_NAME=tf-operator,REPO_OWNER=kubeflow,BUILD_NUMBER=2f58',
      registry: 'gcr.io/kubeflow-images-public',
      versionTag: 'v20180226-403',
      zone: 'us-central1-a',
    },
  },
}