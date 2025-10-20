import subprocess
import os

CHART_PATH = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))


def install():
    subprocess.run(["helm", "install", "kserve-lite", CHART_PATH, "-n", "kserve", "--create-namespace"])

def uninstall():
    subprocess.run(["helm", "uninstall", "kserve-lite", "-n", "kserve"])

def deploy_sample():
    subprocess.run(["kubectl", "apply", "-f", f"{CHART_PATH}/templates/sklearn-sample.yaml", "-n", "kserve"])

if __name__ == "__main__":
    import sys
    cmd = sys.argv[1]
    if cmd == "install":
        install()
    elif cmd == "uninstall":
        uninstall()
    elif cmd == "deploy-sample":
        deploy_sample()
    else:
        print("Unknown command")
