# ModelMesh to KServe Raw Deployment Migration Helper

A bash script that migrates InferenceServices from ModelMesh serving to KServe Raw deployment mode. This tool handles bulk migrations with interactive pagination, template selection, and storage configuration management.

## What it does

- **Migrates models**: Converts ModelMesh InferenceServices to KServe Raw deployment
- **Preserves configuration**: Maintains route exposure, authentication, and storage settings
- **Handles secrets**: Clones and manages storage and authentication secrets
- **Creates resources**: Generates ServingRuntimes, ServiceAccounts, Roles, and RoleBindings
- **Advanced authentication handling**: Properly backs up and recreates authentication resources during preserve-namespace migrations
- **Pagination support**: Interactive navigation for namespaces with hundreds of models
- **Dry-run mode**: Preview changes without applying them
- **Preserve-namespace mode**: In-place migration within the same namespace (destructive) with enhanced backup and rollback capabilities
- **Manual migration**: Generate resources and apply them manually for full control

## Requirements

- `oc` (OpenShift CLI)
- `yq` (YAML processor)
- `openssl`
- Access to both source and target namespaces
- OpenShift cluster login

## Usage

### Standard Migration (to different namespace)
```bash
./modelmesh-to-raw.sh --from-ns <source-namespace> --target-ns <target-namespace> [OPTIONS]
```

### Preserve-Namespace Migration (in-place, destructive)
```bash
./modelmesh-to-raw.sh --from-ns <source-namespace> --preserve-namespace [OPTIONS]
```

### Parameters

| Parameter | Description | Required |
|-----------|-------------|----------|
| `--from-ns` | Source namespace containing ModelMesh InferenceServices | ✅ |
| `--target-ns` | Target namespace for KServe Raw deployment | ✅* |
| `--preserve-namespace` | **⚠️ DESTRUCTIVE**: Migrate in-place within the same namespace | ❌ |
| `--ignore-existing-ns` | Skip check if target namespace already exists | ❌ |
| `--debug` | Show complete processed resources and wait for confirmation | ❌ |
| `--dry-run` | Save all YAML resources to local directory without applying | ❌ |
| `--odh` | Use OpenDataHub template namespace (opendatahub) instead of RHOAI | ❌ |
| `--page-size` | Number of InferenceServices to display per page (default: 10) | ❌ |
| `-h, --help` | Show help message | ❌ |

**\* `--target-ns` is not required when using `--preserve-namespace`**

## Examples

### Basic Migration
```bash
./modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw
```

### Migration with Pagination
```bash
./modelmesh-to-raw.sh --from-ns large-namespace --target-ns kserve-raw --page-size 5
```

### Dry Run Mode
```bash
./modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw --dry-run
```

### Debug Mode with Existing Namespace
```bash
./modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw --ignore-existing-ns --debug
```

### Preserve-Namespace Migration (Destructive, In-Place)
```bash
# ⚠️ WARNING: This is destructive and will replace ModelMesh resources with KServe Raw resources
./modelmesh-to-raw.sh --from-ns modelmesh-serving --preserve-namespace
```

### Preserve-Namespace with Debug Mode
```bash
./modelmesh-to-raw.sh --from-ns modelmesh-serving --preserve-namespace --debug
```

### OpenDataHub Environment
```bash
./modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw --odh
```

## Manual Migration Guide

For complete control over the migration process, you can use dry-run mode to generate all resources and apply them manually:

### Step 1: Generate Resources
```bash
./modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw --dry-run
```

This creates a directory like `migration-dry-run-20241014-143022/` with:
- `original-resources/`: Original ModelMesh resources (for backup/comparison)
- `new-resources/`: New KServe Raw resources to apply

### Step 2: Review Generated Resources
```bash
# Check the directory structure
ls -la migration-dry-run-*/

# Review specific resources
cat migration-dry-run-*/new-resources/inferenceservice/my-model.yaml
cat migration-dry-run-*/new-resources/servingruntime/my-model.yaml
```

### Step 3: Apply Resources Manually
```bash
# Apply all new resources at once
find migration-dry-run-*/new-resources -name '*.yaml' -exec oc apply -f {} \;

# Or apply by category for better control
oc apply -f migration-dry-run-*/new-resources/namespace/
oc apply -f migration-dry-run-*/new-resources/servingruntime/
oc apply -f migration-dry-run-*/new-resources/secret/
oc apply -f migration-dry-run-*/new-resources/serviceaccount/
oc apply -f migration-dry-run-*/new-resources/role/
oc apply -f migration-dry-run-*/new-resources/rolebinding/
oc apply -f migration-dry-run-*/new-resources/inferenceservice/
```

### Step 4: Verify Migration
```bash
# Check all resources are created
oc get inferenceservice -n kserve-raw
oc get servingruntime -n kserve-raw
oc get secret -n kserve-raw
```

### Advantages of Manual Migration
- **Full Control**: Review each resource before applying
- **Selective Application**: Apply only specific resources
- **Custom Modifications**: Edit generated YAMLs before applying
- **Rollback Preparation**: Keep original resources for easy rollback
- **Debugging**: Easier to troubleshoot issues step by step

## Example Output

### Successful Migration
```
ModelMesh to KServe Raw Deployment Migration Helper
==================================================

Source namespace (ModelMesh): modelmesh-serving
Target namespace (KServe Raw): kserve-raw

🔍 Checking OpenShift login status...
✓ Logged in as: developer
✓ Connected to: https://api.cluster.local:6443

🔍 Verifying ModelMesh configuration in source namespace...
✓ ModelMesh is enabled in namespace 'modelmesh-serving'

🚀 Setting up target namespace for KServe Raw deployment...
🏗️ Creating target namespace 'kserve-raw'...
✓ Target namespace 'kserve-raw' created successfully
✓ Dashboard label applied to namespace 'kserve-raw'
✓ modelmesh-enabled label set to false on namespace 'kserve-raw'

🔍 Discovering InferenceServices in source namespace 'modelmesh-serving'...
✓ Found 3 InferenceService(s) in namespace 'modelmesh-serving'

📦 InferenceServices (Page 1/1, showing items 1-3 of 3):
=======================================================================================
[1] Name: mnist-model
    Status: Ready
    Runtime: ovms
    Model Format: onnx
    Storage: s3://my-bucket/mnist/

[2] Name: sklearn-model
    Status: Ready
    Runtime: ovms
    Model Format: sklearn
    Storage: s3://my-bucket/sklearn/

[3] Name: custom-model
    Status: Ready
    Runtime: custom-runtime
    Model Format: tensorflow
    Storage: s3://my-bucket/tensorflow/

🤔 Selection options:
====================
• 'all' - Select all InferenceServices across all pages
• '3 4' - Select specific items by number (e.g., '3 4' to select items 3 and 4)

• 'q' - Quit migration

Your selection: all
✓ Selected all 3 InferenceService(s) for migration

🔧 Preparing serving runtimes for selected models...
✓ All serving runtimes created successfully

🔄 Processing InferenceServices for Raw Deployment migration...
===================================================================
🔧 Transforming InferenceService 'mnist-model' for Raw Deployment...

🔐 Secret Management for InferenceService 'mnist-model'
=======================================================
📁 Current Storage Configuration:
   Path: models/mnist/1/
   URI: s3://my-bucket/mnist/

✓ Selected all 3 InferenceService(s) for migration

🎉 Migration completed successfully!
======================================

📊 Migration Summary:
  • Source namespace: modelmesh-serving (ModelMesh)
  • Target namespace: kserve-raw (KServe Raw)
  • InferenceServices migrated: 3
  • Models: mnist-model, sklearn-model, custom-model

💡 Next steps:
  • Verify your migrated models are working: oc get inferenceservice -n kserve-raw
  • Check ServingRuntimes: oc get servingruntime -n kserve-raw
  • Test model endpoints for functionality

🏁 Migration helper completed!
```

### Pagination Example
```
📦 InferenceServices (Page 1/3, showing items 1-10 of 25):
=======================================================================================
[1] Name: model-001
[2] Name: model-002
...
[10] Name: model-010

🤔 Selection options:
====================
• 'all' - Select all InferenceServices across all pages
• '3 4' - Select specific items by number (e.g., '3 4' to select items 3 and 4)

📄 Navigation:
==============
• 'n' - Next page
• 'l' - Last page
• 'goto:X' - Go to specific page X (e.g., 'goto:3')

• 'q' - Quit migration

Your selection: n
📄 Moving to page 2...

📦 InferenceServices (Page 2/3, showing items 11-20 of 25):
=======================================================================================
[11] Name: model-011
[12] Name: model-012
...
```

### Dry Run Example
```
🏁 Dry-run completed successfully!

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
```

## Features

### Interactive Template Selection
When custom ServingRuntimes are detected, the script presents available templates:
```
🤔 Please select a template for model 'custom-model' from the available ones:
=========================================================================================
    [1] kserve-ovms (OpenVINO Model Server)
    [2] kserve-tensorflow (TensorFlow Serving)
    [3] kserve-pytorch (PyTorch Serving)
    [d] Use default: kserve-ovms (OpenVINO Model Server)
    [m] Enter template name manually

  Your choice (1-3/d/m): 1
```

### Storage Configuration Management
For each model, you can update storage paths for OpenVINO compatibility:
```
📁 Storage Configuration for 'mnist-model':
   Current path: models/mnist/
   Current storageUri: s3://my-bucket/mnist/

🤔 Do you want to update the storage configuration for this model?
   1) Keep current configuration
   2) Enter a new path S3 OpenVINO versioned compatible 'storage.path'
   3) Enter a new URI (storageUri)
   4) Skip this model

Your choice (1/2/3/4): 2
📝 Enter the new storage path (e.g., openvino/mnist/):
New path: models/mnist/1/
✅ Updated path to: models/mnist/1/
```

### Authentication and Route Preservation
The script automatically detects and preserves:
- Route exposure settings (`networking.kserve.io/visibility=exposed`)
- Authentication configuration (`security.opendatahub.io/enable-auth=true`)
- Service account creation and RBAC setup

## Troubleshooting

### Common Issues

**Error: You are not logged into an OpenShift cluster**
```bash
oc login https://your-cluster-url:6443
```

**Error: Source namespace does not have 'modelmesh-enabled' label**
```bash
oc label namespace your-namespace modelmesh-enabled=true
```

**Error: Target namespace already exists**
- Use `--ignore-existing-ns` flag or delete the existing namespace

**Error: Missing dependencies**
- Install required tools: `oc`, `yq`, `openssl`

### Debug Mode
Use `--debug` to see complete YAML resources before applying:
```bash
./modelmesh-to-raw.sh --from-ns source --target-ns target --debug
```

### Preserve-Namespace Mode Issues

**Error: Migration failed during preserve-namespace mode**
- Check the backup directory for rollback instructions: `preserve-namespace-backup-*/ROLLBACK_INSTRUCTIONS.md`
- Use the generated rollback scripts to restore original state

**Warning: Authentication tokens recreated**
- After preserve-namespace migration, authentication tokens are recreated
- Update consumers to use new tokens
- Get new token: `oc get secret token-<model-name>-sa -o jsonpath='{.data.token}' | base64 -d`

**Error: Authentication resources missing after preserve-namespace migration**
- The script now automatically backs up authentication resources before any changes
- Check backup directory: `preserve-namespace-backup-*/original-resources/`
- If resources are missing, use the rollback instructions to restore original state
- This issue should not occur with the enhanced authentication handling

**Error: Old ServingRuntime still exists after migration**
- The script now deletes old ServingRuntimes after all new resources are stable
- Check if migration completed successfully: `oc get servingruntime -n <namespace>`
- Old ServingRuntimes are deleted individually per model to prevent authentication resource loss
- Use debug mode to monitor the deletion process: `--debug`

## Preserve-Namespace Mode Guide

### ⚠️ **IMPORTANT: Destructive Operation Warning**

The `--preserve-namespace` flag performs an **in-place, destructive migration** within the same namespace. This mode completely replaces ModelMesh resources with KServe Raw deployment resources **without the safety of a separate target namespace**.

### When to Use Preserve-Namespace Mode

**Recommended Use Cases:**
- **Namespace constraints**: When you cannot create additional namespaces due to cluster policies
- **Resource quotas**: When cluster resource limits prevent creating new namespaces
- **Network policies**: When existing network configurations are tied to the specific namespace
- **External integrations**: When external systems reference the specific namespace name
- **Simplified management**: When you prefer to maintain the same namespace structure

**⚠️ When NOT to Use:**
- **Production environments** without thorough testing
- **Shared namespaces** with other critical workloads
- **Compliance requirements** that mandate separate migration environments
- **First-time migrations** (use standard mode for initial testing)

### How Preserve-Namespace Mode Works

#### Phase 1: Safety Checks and Warnings
```bash
./modelmesh-to-raw.sh --from-ns modelmesh-serving --preserve-namespace
```

The script will display a comprehensive warning:

```
⚠️  ⚠️  ⚠️ DESTRUCTIVE OPERATION WARNING ⚠️  ⚠️  ⚠️
=================================================

🚨 You have enabled --preserve-namespace mode!

🔥 This will perform the following DESTRUCTIVE actions:
   • Delete existing ModelMesh InferenceServices in 'modelmesh-serving'
   • Remove modelmesh-enabled=true label from namespace
   • Replace with KServe Raw deployment resources

💥 If the migration fails, you will need to restore from backup!

📋 Before proceeding, ensure you have:
   ✓ Tested this migration in a non-production environment
   ✓ Created backups of your InferenceServices
   ✓ Verified you can restore from backup if needed

⏰ The script will generate backups, but restoration is manual!
  ---> 🚨 The authentication token will be recreated, the consumer will need to be updated!

🤔 Do you understand the risks and want to continue? (type 'yes' to proceed):
```

**You must type exactly `yes` to proceed** - any other input will cancel the operation.

#### Phase 2: Backup Creation
The script automatically creates a timestamped backup directory:
```
preserve-namespace-backup-20241015-143022/
├── original-resources/          # Original ModelMesh resources
│   ├── inferenceservice/
│   ├── secret/
│   ├── serviceaccount/
│   ├── role/
│   ├── rolebinding/
│   └── servingruntime/
├── new-resources/              # New KServe Raw resources
│   ├── inferenceservice/
│   ├── secret/
│   ├── serviceaccount/
│   ├── role/
│   ├── rolebinding/
│   └── servingruntime/
└── ROLLBACK_INSTRUCTIONS.md   # Detailed rollback procedures
```

#### Phase 3: Enhanced Destructive Migration Process
The migration follows a carefully orchestrated sequence to prevent resource loss:

1. **Comprehensive backup creation**: All ModelMesh resources are backed up before any changes
2. **Update namespace labels**: `modelmesh-enabled=true` is changed to `modelmesh-enabled=false`
3. **Individual resource migration** (per InferenceService):
   - **Authentication resource backup**: ServiceAccounts, Roles, RoleBindings, and service account tokens are backed up *before* any changes
   - **Create new ServingRuntime**: KServe Raw ServingRuntime is created first
   - **Create new InferenceService**: Transformed for Raw deployment with preserved settings
   - **Create new authentication resources**: ServiceAccounts, Roles, RoleBindings, and tokens for KServe Raw
   - **Delete old ServingRuntime**: Original ModelMesh ServingRuntime is deleted *only after* all new resources are stable
4. **Storage secret migration**: Storage secrets are cloned and transformed for KServe compatibility
5. **Verification**: Each resource is verified after creation before proceeding

### Safety Features and Backup Strategy

#### Automatic Backup Creation
Every preserve-namespace migration creates comprehensive backups:

```bash
# Backup directory structure
preserve-namespace-backup-20241015-143022/
├── original-resources/
│   └── [Complete backup of all original ModelMesh resources]
├── new-resources/
│   └── [All generated KServe Raw resources for review]
└── ROLLBACK_INSTRUCTIONS.md
```

#### Rollback Instructions
The generated `ROLLBACK_INSTRUCTIONS.md` contains step-by-step procedures to restore the original ModelMesh configuration:

```markdown
# Preserve-Namespace Migration Rollback Instructions

## Emergency Rollback Process
1. Delete KServe Raw resources
2. Restore ModelMesh namespace label
3. Restore original InferenceServices
4. Restore original secrets
5. Verify ModelMesh functionality
```

### Step-by-Step Migration Process

#### 1. Pre-Migration Preparation
```bash
# Verify current state
oc get inferenceservice -n modelmesh-serving
oc get servingruntime -n modelmesh-serving
oc get secrets -n modelmesh-serving

# Optional: Create manual backup
oc get all,secrets,serviceaccounts,roles,rolebindings -n modelmesh-serving -o yaml > manual-backup.yaml
```

#### 2. Execute Migration
```bash
# Standard preserve-namespace migration
./modelmesh-to-raw.sh --from-ns modelmesh-serving --preserve-namespace

# With debugging (recommended for first-time use)
./modelmesh-to-raw.sh --from-ns modelmesh-serving --preserve-namespace --debug

# With custom pagination
./modelmesh-to-raw.sh --from-ns modelmesh-serving --preserve-namespace --page-size 5
```

#### 3. Post-Migration Verification
```bash
# Verify new KServe Raw resources
oc get inferenceservice -n modelmesh-serving
oc get servingruntime -n modelmesh-serving
oc get pods -n modelmesh-serving

# Check namespace labels
oc get namespace modelmesh-serving --show-labels

# Verify authentication tokens (if auth was enabled)
oc get secrets -n modelmesh-serving | grep token-

# Test model endpoints
curl -k https://your-model-route/v1/models
```

### Emergency Rollback Procedures

#### Immediate Rollback (if migration fails)
```bash
# Navigate to backup directory
cd preserve-namespace-backup-20241015-143022/

# Follow the automated rollback instructions
cat ROLLBACK_INSTRUCTIONS.md

# Quick rollback commands
# 1. Delete KServe Raw resources
find new-resources/ -name "*.yaml" -exec basename {} .yaml \; | while read resource; do
  oc delete inferenceservice "$resource" -n modelmesh-serving --ignore-not-found
  oc delete servingruntime "$resource" -n modelmesh-serving --ignore-not-found
  oc delete serviceaccount "${resource}-sa" -n modelmesh-serving --ignore-not-found
  oc delete role "${resource}-view-role" -n modelmesh-serving --ignore-not-found
  oc delete rolebinding "${resource}-view" -n modelmesh-serving --ignore-not-found
done

# 2. Restore ModelMesh namespace label
oc label namespace modelmesh-serving modelmesh-enabled=true --overwrite

# 3. Restore original resources
find original-resources/inferenceservice -name "*.yaml" -exec oc apply -f {} \;
find original-resources/secret -name "*.yaml" -exec oc apply -f {} \;
```

#### Verification After Rollback
```bash
# Verify ModelMesh is functional
oc get inferenceservice -n modelmesh-serving
oc get pods -n modelmesh-serving | grep modelmesh
oc logs -l app=modelmesh -n modelmesh-serving
```

### Common Scenarios and Best Practices

#### Scenario 1: Testing in Development
```bash
# Use debug mode for detailed inspection
./modelmesh-to-raw.sh --from-ns dev-modelmesh --preserve-namespace --debug --page-size 3
```

#### Scenario 2: Large-Scale Production Migration
```bash
# Use dry-run first to review changes
./modelmesh-to-raw.sh --from-ns prod-modelmesh --preserve-namespace --dry-run

# Review generated resources
ls -la preserve-namespace-backup-*/
cat preserve-namespace-backup-*/new-resources/inferenceservice/critical-model.yaml

# Execute actual migration
./modelmesh-to-raw.sh --from-ns prod-modelmesh --preserve-namespace
```

#### Scenario 3: Partial Migration with Manual Intervention
```bash
# Generate resources for manual application
./modelmesh-to-raw.sh --from-ns modelmesh-serving --preserve-namespace --dry-run

# Manually modify resources if needed
vi preserve-namespace-backup-*/new-resources/inferenceservice/custom-model.yaml

# Apply manually with full control
oc apply -f preserve-namespace-backup-*/new-resources/
```

### Critical Considerations

#### Authentication Token Recreation
- **Impact**: All service account tokens are recreated during migration
- **Action Required**: Update all consumers with new authentication tokens
- **Get New Tokens**:
  ```bash
  oc get secret token-<model-name>-sa -n modelmesh-serving -o jsonpath='{.data.token}' | base64 -d
  ```

#### Network Policy Updates
- **Impact**: Network policies referencing ModelMesh-specific labels may need updates
- **Action Required**: Review and update network policies for KServe Raw deployment labels

#### Resource Quota Considerations
- **Impact**: Resource requirements may change during migration
- **Action Required**: Ensure namespace has sufficient quota for both old and new resources during transition

#### Monitoring and Alerting
- **Impact**: Monitoring systems may lose track of resources during the destructive phase
- **Action Required**:
  - Temporarily silence alerts during migration window
  - Update monitoring queries for KServe Raw resource labels
  - Verify metrics collection resumes after migration

### Troubleshooting Preserve-Namespace Mode

#### Migration Hangs or Times Out
```bash
# Check for stuck resources
oc get all -n modelmesh-serving
oc get events -n modelmesh-serving --sort-by='.lastTimestamp'

# Force delete stuck resources
oc delete inferenceservice <stuck-model> --grace-period=0 --force
```

#### Partial Migration State
```bash
# Check which phase failed
oc get namespace modelmesh-serving --show-labels
oc get inferenceservice -n modelmesh-serving
oc get servingruntime -n modelmesh-serving

# Use backup to restore to known state
cd preserve-namespace-backup-*/
# Follow ROLLBACK_INSTRUCTIONS.md
```

#### Resource Creation Failures
```bash
# Check resource quotas
oc describe quota -n modelmesh-serving

# Check RBAC permissions
oc auth can-i create inferenceservices --as=system:serviceaccount:modelmesh-serving:default

# Review backup logs
cat preserve-namespace-backup-*/migration.log  # if log file exists
```

### Best Practices Summary

#### Before Migration
- ✅ **Test in non-production environment first**
- ✅ **Verify sufficient resource quotas**
- ✅ **Document current authentication tokens**
- ✅ **Notify consumers of planned downtime**
- ✅ **Create manual backups if required by policy**

#### During Migration
- ✅ **Use debug mode for critical migrations**
- ✅ **Monitor cluster resource usage**
- ✅ **Keep backup directory accessible**
- ✅ **Have rollback procedures ready**

#### After Migration
- ✅ **Verify all models are functional**
- ✅ **Update authentication tokens in consumer applications**
- ✅ **Update monitoring and alerting configurations**
- ✅ **Archive backup directories according to retention policy**
- ✅ **Document any manual changes made during migration**

## Help

```bash
./modelmesh-to-raw.sh --help
```

```
ModelMesh to KServe Raw Deployment Migration Helper

USAGE:
    ./modelmesh-to-raw.sh --from-ns <source-namespace> --target-ns <target-namespace> [OPTIONS]
    ./modelmesh-to-raw.sh --from-ns <source-namespace> --preserve-namespace [OPTIONS]

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
    ./modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw

    # Preserve-namespace migration (destructive, in-place)
    ./modelmesh-to-raw.sh --from-ns modelmesh-serving --preserve-namespace

    # Dry-run mode for manual migration
    ./modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw --dry-run

    # With pagination and debugging
    ./modelmesh-to-raw.sh --from-ns large-ns --target-ns kserve-raw --page-size 20 --debug

    # OpenDataHub environment
    ./modelmesh-to-raw.sh --from-ns modelmesh-serving --target-ns kserve-raw --odh

REQUIREMENTS:
    - oc (OpenShift CLI)
    - yq (YAML processor)
    - Access to both source and target namespaces (or source namespace for --preserve-namespace)
```