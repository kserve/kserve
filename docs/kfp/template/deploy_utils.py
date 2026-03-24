import subprocess
import re
import os


def prepare_env_and_deploy(args):
    """
    Prepare the OpenShift environment and deploy KServe manifests and dependencies.
    `args` is a dict of parsed CLI arguments.
    """
    if args["action"] == "delete":
        print(f"Getting the current project...")
        result = subprocess.run(
            ["oc", "project"], capture_output=True, text=True, check=True
        )
        result = result.stdout
        namespace = re.search(r'"(.*?)"', result).group(1)
        args["namespace"] = namespace
        os.environ["NAMESPACE"] = namespace
    else:
        print(f"Checking if OpenShift project '{args['namespace']}' exists...")
        result = subprocess.run(f"oc get project {args['namespace']}", shell=True)
        if result.returncode == 0:
            print(f"Project '{args['namespace']}' already exists. Skipping creation.")
        else:
            print(f"Creating new OpenShift project: {args['namespace']}")
            subprocess.run(
                f"oc new-project {args['namespace']}", shell=True, check=True
            )

    _handle_create_action(args)

    print("Installing Python dependencies...")
    subprocess.run("pip install -r requirements.txt", shell=True, check=True)

    print(f"Deploying manifests with action={args['action']}...")
    cmd = "kustomize build manifests | envsubst | oc apply -f -"
    subprocess.run(cmd, shell=True, check=True)


def _handle_create_action(args):
    """
    Logic to handle the possible actions:
    If action is 'create':  if InferenceService exists, change action to 'update'
    If action is 'update': if InferenceService does not exist, change action to 'apply'
    """
    if args["action"] == "create":
        print(
            f"Checking if InferenceService '{args['model_name']}' exists in namespace '{args['namespace']}'..."
        )
        result = subprocess.run(
            f"oc get inferenceservice {args['model_name']} -n {args['namespace']}",
            shell=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        if result.returncode == 0:
            print(
                f"InferenceService '{args['model_name']}' already exists. Switching action to 'update'."
            )
            args["action"] = "update"
        else:
            print(
                f"InferenceService '{args['model_name']}' does not exist. Proceeding with 'create'."
            )

    if args["action"] == "update":
        print(
            f"Checking if InferenceService '{args['model_name']}' exists in namespace '{args['namespace']}'..."
        )
        result = subprocess.run(
            f"oc get inferenceservice {args['model_name']} -n {args['namespace']}",
            shell=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        if result.returncode != 0:
            print(
                f"InferenceService '{args['model_name']}' does not exist. Switching action to 'apply'."
            )
            args["action"] = "apply"
        else:
            print(
                f"InferenceService '{args['model_name']}' exists. Proceeding with 'update'."
            )


def compile_pipeline(action):
    """
    Compile the KFP pipeline Python script into a YAML definition.
    """
    print("Compiling the pipeline...")
    if action == "delete":
        subprocess.run(
            "kfp dsl compile --py pipeline/delete_model_pipeline.py --output pipeline.yaml",
            shell=True,
            check=True,
        )
        return
    subprocess.run(
        "kfp dsl compile --py pipeline/deploy_model_pipeline.py --output pipeline.yaml",
        shell=True,
        check=True,
    )
