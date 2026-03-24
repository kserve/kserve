# Using Devspace

[Devspace site](https://devspace.sh)

## Prerequisites to use devspace

**These get copied into the running container and are defined in the devspace.yaml file.**

1. Binaries
    - devspace: [Devspace install instructions](https://www.devspace.sh/docs/getting-started/installation)
    - kubectl
    - helm
    - git
2. kubeconfig in `~/.kube/config` with current context set to cluster you want to use

## Using Devspace

### If no devspace.yaml or devspace.sh files exist you will need to generate them

**Note: This has already been run in this repo and should not be needed.**

- Run `devspace init`

This will also add .devspace to your .gitignore file. If it doesn't, please add it.

#### The devspace.yaml file (config file) defines the following:

[Devspace Configuration](https://www.devspace.sh/docs/configuration/reference)

- Container Image that will be used when creating the devspace container
- The labelselector that will be used to choose which deployment it will be replacing
- The binaries that it will copy into the container
- The available pipelines a user can run `devspace run-pipeline ${pipeline_name}`
- ENV Vars that can be defined under the **vars** section
- The path to the manifest and what tool to use to deploy (helm or kustomize)
    - in this repos case `../config/manager` is the path `kustomize` is being used

### Common commands

[More Development Commands](https://www.devspace.sh/docs/getting-started/development)

**NOTE:** You don't have to run the use context and use namespace commands if you haven't misconfigured devspace since the last time you used it with this repo.

For regular development you usually run `devspace use context`, then `devspace use namespace`, then `devspace dev`

- `devspace use context` to select your kubernetes cluster in the case you have multiple in your kubeconfig file
- `devspace use namespace` change what namepsace you want to deploy your container into e.g. `devspace use namespace opendatahub` for this project
- `devspace dev` main command used to start up the devspace container
- `devspace run-pipeline ${pipeline_name}` to run a specific pipeline e.g. `debug` pipeline that was configured for this project
**NOTE:** The default image uses go1.24. If you need to use go1.22, you can run with the flag `--profile go1.22` ex: `$ devspace run-pipeline debug --profile go1.22` 

[More Cleanup Commands](https://www.devspace.sh/docs/getting-started/cleanup)

- `devspace purge` used to delete your project from the cluster
- `devspace reset pods` to reverse `start_dev` command that devspace runes within a pipeline


### Debugging Kserve

##### Prereuisites 
- You need to have vscode installed on your machine
- vscode must be [executable](https://code.visualstudio.com/docs/configure/command-line) via cli on your machine 

##### Steps to start remote debugger
- `devspace run-pipeline debug` to start the debug pipeline
  - This will start the devspace container and open a vscode server instance that is connected to the container
- Configure the vscode launch.json to run and debug kserve
  - Examples : 
    ```{
        "version": "0.2.0",
        "configurations": [
          {
            "name": "Run KServe Manager",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${workspaceFolder}/cmd/manager",
            "env": {},
            "args": []
          },
          {
            "name": "Debug KServe Manager",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/cmd/manager",
            "env": {},
            "args": []
          }
        ]
      }
      ```
  - You can now set breakpoints in the vscode window and start debugging by selecting `Run` -> `Start Debugging`

> Note: When the vscode server is initially started you might need to install the vscode-go extension, as well as 
> some dependencies vscode needs to execute go programs. Try to run the program and monitor the `Debug` window in vscode.
> You might need to run it a couple of times before it starts running, it can be confirmed by seeing the Kserve logs in the
> Debug window. At this point, you are ready to start debugging. 
