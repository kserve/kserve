/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/containers/buildah"
	"github.com/google/go-containerregistry/pkg/name"
	logging "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.podman.io/storage/pkg/unshare"

	"github.com/kserve/kserve/mcv/pkg/client"
	"github.com/kserve/kserve/mcv/pkg/config"
	"github.com/kserve/kserve/mcv/pkg/imgbuild"
	"github.com/kserve/kserve/mcv/pkg/logformat"
	"github.com/kserve/kserve/mcv/pkg/utils"
)

const (
	exitNormal       = 0
	exitExtractError = 1
	exitCreateError  = 2
	exitLogError     = 3
	version          = "1.0.0" // Application version
)

func main() {
	// InitReexec must run before anything else. When buildah re-executes the
	// binary as a copier subprocess, it enters here, does its work, and returns
	// true — so we exit immediately. For the normal (non-reexec) path it
	// returns false and execution continues as usual.
	if buildah.InitReexec() {
		return
	}

	initializeLogging()

	if _, err := config.Initialize(config.ConfDir); err != nil {
		logFatal("Error initializing config", err, exitLogError)
	}

	cmd := buildRootCommand()
	if err := cmd.Execute(); err != nil {
		logFatal("Error executing command", err, exitLogError)
	}
}

func initializeLogging() {
	logging.SetReportCaller(true)
	logging.SetFormatter(logformat.Default)
}

func logFatal(message string, err error, exitCode int) {
	logging.Errorf("%s: %v", message, err)
	os.Exit(exitCode)
}

func buildRootCommand() *cobra.Command {
	var imageName, cacheDirName, logLevel, builder string
	var createFlag, extractFlag, baremetalFlag, noGPUFlag, checkCompatFlag, gpuInfoFlag, stubFlag, versionFlag bool
	var timeout int

	cmd := &cobra.Command{
		Use:   "mcv",
		Short: "A GPU Kernel runtime container image management utility",
		Long: `mcv is a utility for managing GPU kernel runtime container images.
It supports creating OCI images from cache directories, extracting caches from images,
and performing hardware compatibility checks.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if err := logformat.ConfigureLogging(logLevel); err != nil {
				logFatal("Error configuring logging", err, exitLogError)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			if versionFlag {
				fmt.Printf("mcv version %s\n", version)
				os.Exit(exitNormal)
			}
			handleRunCommand(imageName, cacheDirName, logLevel, builder, createFlag, extractFlag, baremetalFlag, noGPUFlag, checkCompatFlag, gpuInfoFlag, stubFlag, timeout)
		},
	}

	addFlags(cmd, &imageName, &cacheDirName, &logLevel, &builder, &createFlag, &extractFlag, &baremetalFlag, &noGPUFlag, &checkCompatFlag, &gpuInfoFlag, &stubFlag, &timeout)
	cmd.Flags().BoolVar(&versionFlag, "version", false, "Display the version of the application")
	return cmd
}

func addFlags(cmd *cobra.Command, imageName, cacheDirName, logLevel, builder *string, createFlag, extractFlag, baremetalFlag, noGPUFlag, checkCompatFlag, gpuInfoFlag, stubFlag *bool, timeout *int) {
	// Image operations
	cmd.Flags().StringVarP(imageName, "image", "i", "", "OCI image name (required for create, extract, check-compat)")
	cmd.Flags().StringVarP(cacheDirName, "dir", "d", "", "Triton/vLLM cache directory path")

	// Actions (mutually exclusive main operations)
	cmd.Flags().BoolVarP(createFlag, "create", "c", false, "Create OCI image from cache directory")
	cmd.Flags().BoolVarP(extractFlag, "extract", "e", false, "Extract Triton/vLLM cache from OCI image")

	// Information commands
	cmd.Flags().BoolVar(gpuInfoFlag, "gpu-info", false, "Display GPU-specific information")
	cmd.Flags().BoolVar(checkCompatFlag, "check-compat", false, "Check GPU compatibility with specified image")

	// Configuration options
	cmd.Flags().StringVarP(logLevel, "log-level", "l", "info", "Set logging verbosity (debug, info, warning, error)")
	cmd.Flags().BoolVarP(baremetalFlag, "baremetal", "b", false, "Enable detailed baremetal preflight checks")
	cmd.Flags().BoolVar(noGPUFlag, "no-gpu", false, "Disable GPU detection and preflight checks (for testing)")
	cmd.Flags().BoolVar(stubFlag, "stub", false, "Use mock/stub data for hardware info (for testing)")
	cmd.Flags().StringVar(builder, "builder", "", "Specify the builder to use (buildah or docker)")
	cmd.Flags().IntVarP(timeout, "timeout", "t", 10, "Timeout in minutes for hardware detection operations (0 = disable timeout)")

	// Mark mutually exclusive flags
	cmd.MarkFlagsMutuallyExclusive("create", "extract")
	cmd.MarkFlagsMutuallyExclusive("no-gpu", "gpu-info")
	cmd.MarkFlagsMutuallyExclusive("no-gpu", "check-compat")
}

func handleRunCommand(imageName, cacheDirName, logLevel, builder string, createFlag, extractFlag, baremetalFlag, noGPUFlag, checkCompatFlag, gpuInfoFlag, stubFlag bool, timeout int) {
	// Validate flag combinations
	if err := validateFlagCombinations(createFlag, extractFlag, gpuInfoFlag, checkCompatFlag, imageName, cacheDirName, stubFlag); err != nil {
		logging.Error(err)
		os.Exit(exitLogError)
	}

	if createFlag {
		runCreate(imageName, cacheDirName, builder)
		return
	}

	configureBoolFlags(baremetalFlag, noGPUFlag, stubFlag)

	if gpuInfoFlag {
		handleGPUInfo(timeout)
		return
	}

	if checkCompatFlag {
		handleCheckCompat(imageName)
		return
	}

	if extractFlag {
		runExtract(imageName, cacheDirName, logLevel, baremetalFlag)
		return
	}

	// If no action is specified, show help
	logging.Error("No action specified. Use --help to see available options.")
	os.Exit(exitNormal)
}

func validateFlagCombinations(createFlag, extractFlag, gpuInfoFlag, checkCompatFlag bool, imageName, cacheDirName string, stubFlag bool) error {
	actionCount := 0
	if createFlag {
		actionCount++
	}
	if extractFlag {
		actionCount++
	}
	if gpuInfoFlag {
		actionCount++
	}
	if checkCompatFlag {
		actionCount++
	}

	if actionCount > 1 {
		return errors.New("only one action flag can be specified at a time")
	}

	if actionCount == 0 {
		return errors.New("no action specified. Use --help to see available options")
	}

	// Image name requirements
	if (createFlag || extractFlag || checkCompatFlag) && imageName == "" {
		return errors.New("--image is required when using --create, --extract, or --check-compat")
	}

	// Validate imageName against imageNameRegex
	if imageName != "" {
		_, err := name.ParseReference(imageName, name.StrictValidation)
		if err != nil {
			return fmt.Errorf("error validating image name: %w", err)
		}
	}

	// Cache directory requirements
	if createFlag && cacheDirName == "" {
		return errors.New("--dir is required when using --create")
	}

	// Stub flag validation
	if stubFlag && !gpuInfoFlag {
		return errors.New("--stub can only be used with --gpu-info")
	}

	return nil
}

func handleGPUInfo(timeout int) {
	stub := config.IsStubEnabled()
	summary, err := client.GetSystemGPUInfo(client.HwOptions{EnableStub: &stub, Timeout: timeout})
	if err != nil && summary == nil {
		logging.Errorf("Error getting system hardware: %v", err)
		os.Exit(exitLogError)
	}
	client.PrintGPUSummary(summary)

	os.Exit(exitNormal)
}

func handleCheckCompat(imageName string) {
	if imageName == "" {
		logging.Error("--image is required with --check-compat")
		os.Exit(exitLogError)
	}

	matched, unmatched, err := client.PreflightCheck(imageName)
	if err != nil {
		logging.Errorf("Preflight check failed: %v", err)
	}

	if len(matched) > 0 {
		logging.Debugf("Compatible GPU(s) found (%d):", len(matched))
		logging.Debugf("IDs: %v", matched)
	} else {
		logging.Warn("No compatible GPUs found for the image.")
	}

	if len(unmatched) > 0 {
		logging.Debugf("Incompatible GPU(s) found (%d):", len(unmatched))
		logging.Debugf("IDs: %v", unmatched)
	}

	if err != nil || len(matched) == 0 {
		logging.Warn("Exiting: no compatible GPU(s) detected or error occurred during compatibility check")
		os.Exit(exitExtractError)
	}
	os.Exit(exitNormal)
}

func configureBoolFlags(baremetalFlag, noGPUFlag, stub bool) {
	config.SetEnabledBaremetal(baremetalFlag)
	config.SetEnabledStub(stub)
	config.SetEnabledGPU(!noGPUFlag)

	logging.Debugf("baremetalFlag %v", baremetalFlag)
	logging.Debugf("stub %v", stub)
	logging.Debugf("noGPUFlag %v", noGPUFlag)

	if noGPUFlag {
		logging.Debug("GPU checks disabled: running in no-GPU mode (--no-gpu)")
		return
	}
}

func runCreate(imageName, cacheDir, builder string) {
	// MaybeReexecUsingUserNamespace is only needed for image creation via buildah,
	// which requires re-executing the binary inside a new user namespace to perform
	// rootless container storage operations (mount, overlay, etc.).
	unshare.MaybeReexecUsingUserNamespace(false)

	// Check if the cache directory exists
	if _, err := utils.FilePathExists(cacheDir); err != nil {
		logging.Errorf("Error checking cache file path: %v", err)
		os.Exit(exitCreateError)
	}

	// Initialize the image builder
	var builderInstance imgbuild.ImageBuilder
	var err error
	if builder == "" {
		// Default to old behavior: auto-detect builder
		builderInstance, err = imgbuild.New()
	} else {
		builderInstance, err = imgbuild.NewWithBuilder(builder)
	}

	if err != nil {
		logging.Errorf("Failed to create builder: %v", err)
		os.Exit(exitCreateError)
	}

	// Create the OCI image
	if err := builderInstance.CreateImage(imageName, cacheDir); err != nil {
		logging.Errorf("Failed to create the OCI image: %v", err)
		os.Exit(exitCreateError)
	}

	logging.Info("OCI image created successfully.")
}

func runExtract(imageName, cacheDir, logLevel string, baremetalFlag bool) {
	gpuEnabled := config.IsGPUEnabled()
	opts := client.Options{
		ImageName:       imageName,
		CacheDir:        cacheDir,
		EnableGPU:       &gpuEnabled,
		LogLevel:        logLevel,
		EnableBaremetal: &baremetalFlag,
	}
	if _, _, err := client.ExtractCache(opts); err != nil {
		logging.Errorf("Error extracting image: %v", err)
		os.Exit(exitExtractError)
	}
}
