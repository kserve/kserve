# OpenDataHub \- ISVC Migration Guide

# 1\. Overview

**NOTE: This migration MUST be performed on ODH 2.X. It is a prerequisite for upgrading to ODH 3.0.**

## 1.1. Purpose of this Migration

In ODH 3.0 we are deprecating the usage of ModelMesh and Serverless Deployment Mode in Kserve. With these deprecations, we will need users to migrate their existing InferenceServices to RawDeployment (Standard in the UI) Mode. This document will describe the necessary steps for users to migrate their existing InferenceServices to RawDeployment with both automated and manual steps. They will then need to manually validate that the new InferenceServices are up and running correctly, then manually delete the old InferenceServices once the new RawDeployment mode InferenceServices are validated. The ODH team is providing these instructions as a best effort to help migrate users to RawDeployment.

## 1.2 Serverless \-\> Raw Helper Overview

1. Discovers InferenceServices with 'Serverless' deployment mode  
2. Allows interactive selection of which models to convert  
3. For each selected InferenceService:  
   1. Exports original InferenceService and ServingRuntime configurations  
   2. Creates new '-raw' versions with RawDeployment mode  
   3. Handles authentication resources (ServiceAccount, Role, RoleBinding, Secret)  
   4. Applies all transformed resources to the cluster (unless \--dry-run is used)

    4\. Optionally preserves exported files for review (User will be prompted)

**Note**: If the namespace contains custom configurations like Cluster Storage, Permissions, etc, it needs to be replaced manually to the new namespace before proceeding with the migration.

Please download these scripts  and give them execution permission

```shell
# serverless
https://github.com/opendatahub-io/kserve/tree/master/migration-helper/serverless-to-raw.sh
chmod +x serverless-to-raw.sh Orcurl https://raw.githubusercontent.com/opendatahub-io/kserve/refs/heads/master/migration-helper/serverless-to-raw.sh -o serverless-to-raw.sh


# modelmesh
$ curl https://raw.githubusercontent.com/opendatahub-io/kserve/refs/heads/master/migration-helper/modelmesh-to-raw -o modelmesh-to-raw.sh
# then
$ bash modelmesh-to-raw.sh --help
```

## 1.3 ModelMesh \-\> Raw Script Overview

The helper script automates the migration of AI/ML models from ModelMesh serving to KServe Raw Deployment mode in OpenShift environments. It handles the complete transformation of inference services, including authentication resources, storage configurations, and runtime settings. It also allows you to override the model storage location in case you already have the new model to be used, compatible with the OpenVINO directory structure as described [here](https://docs.google.com/document/d/1QO3qOCBm0bi7d-nzF6oYtWT3UZ3zcvbaIA0XrcIVM8M/edit?tab=t.0#heading=h.n120jxwvztle). If you don't want to update the model location in the storage, the script will handle it by annotating the migrated InferenceService properly, so no changes in the storage location should be needed.

1. **Validates environments** and ensures proper access  
2. **Creates the target namespace** with appropriate configurations  
3. **Creates serving runtimes** to ModelMesh to KServe  
4. **Transforms** InferenceService for Raw Deployment mode  
5. **Handles** authentication resources (when enabled on ModelMesh)  
6. **Exposes** routes (when exposed on ModelMesh)  
7. **Copies** storage secrets and configurations  
8. **Preserves** model configurations while adapting to a new deployment mode  
9. **Manual migration**, generate resources, and apply them manually for full control

During the migration helper execution, the user will be prompted to:

* The models he wants to migrate  
  * Single, multiple, or all.  
    * For every model, the process is started over.  
* The storage configuration   
* If the migration tool detects that the Serving Runtime is used, the user would need to make sure to have it created previously, following the documentation, so it can be properly used on the Dashboard.

About the Token generation, this can't be reused. For such a scenario, when authentication is enabled, a new token will be issued.

# 2\. Prerequisites

NOTE: Scripts will verify that the user running the script has all of these prerequisites before executing.

* OpenShift CLI (oc) \- logged into target cluster  
* yq (YAML processor) \- for YAML manipulation  
* jq (JSON processor) \- for JSON manipulation  
* Appropriate RBAC permissions in target namespace (not needed for \--dry-run)

# 3\. Running the Script

## 3.1. How to Use the Serverless \-\> Raw Script

### 3.1.1. Dry Run Mode

`./serverless-to-raw.sh --dry-run Will only generate files, but not apply them to your cluster`

### 3.1.2. Normal Usage

`./serverless-to-raw.sh -n <namespace>`  
`<namespace> is optional, it will default to the current project when a user runs oc project`

### 3.1.3. Help Message

`./serverless-to-raw.sh -h For detailed information about the script, what it does, and how to run it`

## 3.2. How to Use the ModelMesh \-\> Raw Script

To see the supported parameters, use *\--help* flag:

```c
ModelMesh to KServe Raw Deployment Migration Helper

USAGE:
    bash modelmesh-to-raw.sh --from-ns <source-namespace> --target-ns <target-namespace> [OPTIONS]
    bash modelmesh-to-raw.sh --from-ns <source-namespace> --preserve-namespace [OPTIONS]

PARAMETERS:
    --from-ns <namespace>      Source namespace containing ModelMesh InferenceServices
    --target-ns <namespace>    Target namespace for KServe Raw deployment (not required with --preserve-namespace)
    --preserve-namespace       ⚠️ DESTRUCTIVE: Migrate in-place within the same namespace
    --ignore-existing-ns       Skip check if target namespace already exists
    --debug                    Show complete processed resources and wait for user confirmation
    --dry-run                  Save all YAML resources to local directory without applying them
    --odh                      Use OpenDataHub template namespace (opendatahub) instead of RHOAI (redhat-ods-applications)
    --page-size <number>       Number of InferenceServices to display per page (default: 10)
    -h, --help                 Show this help message

DESCRIPTION:
    This script migrates InferenceServices from ModelMesh to KServe Raw deployment.

    Standard mode: Copies models from the source namespace to a target namespace.
    Preserve-namespace mode: Migrates in-place within the same namespace (destructive).

    For namespaces with many InferenceServices, use --page-size to control pagination.

EXAMPLES:
    # Standard migration to different namespace
    modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw

    # Preserve-namespace migration (destructive, in-place)
    modelmesh-to-raw.sh --from-ns modelmesh-serving --preserve-namespace

    # Dry-run mode for manual migration
    modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw --dry-run

    # With pagination and debugging
    modelmesh-to-raw.sh --from-ns large-ns --target-ns kserve-raw --page-size 20 --debug

    # OpenDataHub environment
    modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw --odh

REQUIREMENTS:
    - oc (OpenShift CLI)
    - yq (YAML processor)
    - Access to both source and target namespaces (or source namespace for --preserve-namespace)

```

As example, a command to migrate models from one namespace to another:

```c
$ modelmesh-to-raw.sh --from-ns public-models --target-ns public-kserve
```

# 4\. Manual migration

For manual migration, there are two ways of moving forward, one is redeploying the target Model using the ODH Dashboard by following [these](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/2.24/html/deploying_models/index) steps from Official docs.   
And the second option is to use the resources created by the Migration Helper tools, as described below.

## 4.1 \- Serverless

The Serverless migration tool provides the *dry-run* parameter, it will collect all resources that need to be applied in the target namespace. To start, execute the following command to collect the resources that need to be applied.

1. `Run the dry-run mode`  
   1. [`serverless-to-raw.sh`](http://serverless-to-raw.sh) `--dry-run -n <namespace>`  
2. `If choosing the same name files, then you need to delete the old ISVC via the UI first, then delete the <isvc_name>-<namespace> Route in the istio-system namespace.`   
   `NOTE: Only needed if doing step 3 option a`  
   1. `oc delete route <isvc_name>-<namespace> -n istio-system`  
3. `Choose which files you want to apply`  
   1. `oc apply -f <isvc_name>/raw/`   
   2. `oc apply -f <isvc_name>/raw-original-names/`

Folder structure

```shell
<inference-service-name>/
    ├── original/              # Original exported resources
    ├── raw/                   # Transformed resources for raw deployment (with -raw suffix)
    └── raw-original-names/    # Transformed resources with original names (for in-place replacement)

```

## 4.2 \- Modelmesh

The Modelmesh migration tool provides the *dry-run* parameter, which will collect all resources that need to be applied in the target namespace. To start, execute the following command to collect the resources that will be applied.

```shell

📋 DRY-RUN SUMMARY
==================

All YAML resources have been saved to: migration-dry-run-20251014-124606

📊 Resources saved:
  • Original ModelMesh resources:  6 files
  • New KServe Raw resources:      7 files

📂 Directory structure:
  migration-dry-run-20251014-124606
  ├── new-resources
  │   ├── inferenceservice
  │   │   └── mnist-route.yaml
  │   ├── namespace
  │   ├── role
  │   │   └── mnist-route-view-role.yaml
  │   ├── rolebinding
  │   │   └── mnist-route-view.yaml
  │   ├── secret
  │   │   ├── localminio.yaml
  │   │   └── token-mnist-route-sa.yaml
  │   ├── serviceaccount
  │   │   └── mnist-route-sa.yaml
  │   └── servingruntime
  │       └── mnist-route.yaml
  └── original-resources
      ├── inferenceservice
      │   └── mnist-route-original.yaml
      ├── namespace
      ├── role
      │   └── ovms-mm-auth-view-role-original.yaml
      ├── rolebinding
      │   └── ovms-mm-auth-view-original.yaml
      ├── secret
      │   └── localminio-original.yaml
      ├── serviceaccount
      │   └── ovms-mm-auth-sa-original.yaml
      └── servingruntime
          └── ovms-mm-auth-original.yaml
  
  17 directories, 13 files
```

And, as next steps, review and apply the files manually in the target namespace:

```c
💡 Next steps:
  1. Review the generated YAML files in migration-dry-run-20251014-120509
  2. Compare original vs new resources to understand the migration changes
  3. When ready, apply the resources manually:
     find migration-dry-run-20251014-124606/new-resources -name '*.yaml' -exec oc apply -f {} \;
  4. Or re-run this script without --dry-run to perform the actual migration
```

Note: Repeat this process per model.

# 5\. ISVC Validation

Users need to ensure the new raw ISVC is functioning correctly after migration.

1. Make sure the inferenceService is Ready with an endpoint available.

   External endpoint validation:

   Grab the token from the Openshift AI Dashboard \-\> Data Science Projects \-\> \<your project\> \-\> Model Deployment Name drop down arrow \-\> copy token

```shell
export TOKEN=<your_token>curl -k -X POST   -H "Authorization: Bearer $TOKEN"   -H "Content-Type: application/json" <your_endpoint_for_infer> -d @<path_to_request>.json 

e.g. curl -k -X POST   -H "Authorization: Bearer $TOKEN"   -H "Content-Type: application/json"   https://advanced-raw-testing.apps-crc.testing/v2/models/advanced-raw/infer   -d @/home/user/request.json
```

 

## 5.1. Troubleshooting ISVC Startup Issues

### 5.1.1. Resource Check

If ISVC is not starting, verify that you have sufficient resources. This could mean only converting a few InferenceServices at a time rather than converting all at the same time.

### 5.1.2. Resource Remediation for Serverless

If you attempt to convert only one and the InferenceService still doesn’t come up due to insufficient resources, pause the Serverless (Advanced) InferenceService that you are currently attempting to convert to free up resources. The new RawDeployment (Standard) mode InferenceService will come up shortly, and you can continue with validation.

### 5.1.3. ModelMesh Deviation

This subsection will note that ModelMesh does not support stopping ISVCs.

# 6\. Removing Old ISVCs and Updating Downstream Endpoints and Tokens

Once Section 5 is complete and the Raw Deployment ISVCs are validated, update your downstream applications with the new tokens and endpoints. Afterward, you can remove the old ISVCs if not done so via the helper script already.

# 7\. Editing the Data Science Cluster (DSC)

**Prerequisite**: Only RawDeployment (Standard) InferenceServices are in your cluster  
Once your cluster no longer has any Serverless or ModelMesh InferenceServices you can proceed with this step.

In the UI, go to OperatorHub \-\> ODH \-\> DataScienceClusters \-\> Edit your DataScienceCluster  
Then perform steps 6.1 through 6.4. Keep in mind, all steps might not apply to all users.

## 7.1. Default Deployment Mode

If set to Serverless, change `defaultDeploymentMode: RawDeployment`

## 7.2. Managed Serverless

If knative-serving managementState is set to Managed, change to Removed

## 7.3. Unmanaged Serverless

If knative-serving managementState is set to `Unmanaged`, then no action is needed.

## 7.4. ModelMesh

Set ModelMesh to `Removed`

## 7.5. Hit Save after you’ve completed all sections from 7.1 \- 7.4that applied to you

# 8\. Editing the Data Science Cluster Initialization (DSCI)

In the UI, go to OperatorHub \-\> ODH \-\> DSCInitializations \-\> Edit your DSCInitialization

## 8.1. Option A: Managed OSSM

In the Spec, if serviceMesh \-\> managementState is set to `Managed,` set it to `Removed` and click Save. Fully removing the serviceMesh section in the DSCI is not supported, the only supported options will be Removed or Unmanaged.

## 8.1. Option B: Unmanaged OSSM (Critical Step)

If on OSSM2 Users will NEED to upgrade to OSSM3 before upgrading to ODH 3.0.

# 9\. Uninstall Operators

## 9.1 Uninstall Serverless Operator (Only if Previously Set to Removed in DataScienceCluster)

Go to Operators \-\> Installed Operators \-\> Red Hat OpenShift Serverless \-\>  Uninstall Operator

## 9.2. Uninstall OSSM Operator (Only if Previously set to Removed in DSCInitialization)

Go to Operators \-\> Installed Operators \-\> Red Hat OpenShift Service Mesh 2 \-\> Uninstall Operator. 

## 9.3. Uninstall Authorino Operator Go to Operators \-\> Installed Operators \-\> Authorino Operator \-\> Uninstall Operator

# 10\. Proceed with other ODH 3.0 Upgrade tasks It is critical to either fully remove OSSM2 or upgrade to OSSM3 to be able to upgrade to ODH 3.0 smoothly. If done so, then you have successfully migrated to use only RawDeployment Mode InferenceServices and you can proceed with next steps.


