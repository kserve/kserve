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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/buildah"
	"github.com/google/go-containerregistry/pkg/name"
	logging "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.podman.io/storage/pkg/unshare"

	"github.com/kserve/kserve/kernelcache/mcv/pkg/client"
	"github.com/kserve/kserve/kernelcache/mcv/pkg/config"
	"github.com/kserve/kserve/kernelcache/mcv/pkg/imgbuild"
	"github.com/kserve/kserve/kernelcache/mcv/pkg/logformat"
	cachesnapshot "github.com/kserve/kserve/kernelcache/mcv/pkg/snapshot"
	"github.com/kserve/kserve/kernelcache/mcv/pkg/utils"
)

const (
	exitNormal        = 0
	exitExtractError  = 1
	exitDeltaError    = 1
	exitCreateError   = 2
	exitLogError      = 3
	exitSnapshotError = 4
	version           = "1.0.0" // Application version
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
	var imageName, cacheDirName, logLevel, builder, resultPath, snapshotPath string
	var excludedDirectories []string
	var createFlag, extractFlag, snapshotFlag, deltaFromSnapshotFlag, baremetalFlag, noGPUFlag, checkCompatFlag, gpuInfoFlag, stubFlag, versionFlag bool
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
			handleRunCommand(imageName, cacheDirName, logLevel, builder, resultPath, snapshotPath, excludedDirectories, createFlag, extractFlag, snapshotFlag, deltaFromSnapshotFlag, baremetalFlag, noGPUFlag, checkCompatFlag, gpuInfoFlag, stubFlag, timeout)
		},
	}

	addFlags(cmd, &imageName, &cacheDirName, &logLevel, &builder, &resultPath, &snapshotPath, &excludedDirectories, &createFlag, &extractFlag, &snapshotFlag, &deltaFromSnapshotFlag, &baremetalFlag, &noGPUFlag, &checkCompatFlag, &gpuInfoFlag, &stubFlag, &timeout)
	cmd.Flags().BoolVar(&versionFlag, "version", false, "Display the version of the application")
	return cmd
}

func addFlags(cmd *cobra.Command, imageName, cacheDirName, logLevel, builder, resultPath, snapshotPath *string, excludedDirectories *[]string, createFlag, extractFlag, snapshotFlag, deltaFromSnapshotFlag, baremetalFlag, noGPUFlag, checkCompatFlag, gpuInfoFlag, stubFlag *bool, timeout *int) {
	// Image operations
	cmd.Flags().StringVarP(imageName, "image", "i", "", "OCI image name (required for create, extract, check-compat)")
	cmd.Flags().StringVarP(cacheDirName, "dir", "d", "", "Triton/vLLM cache directory path")

	// Actions (mutually exclusive main operations)
	cmd.Flags().BoolVarP(createFlag, "create", "c", false, "Create OCI image from cache directory")
	cmd.Flags().BoolVarP(extractFlag, "extract", "e", false, "Extract Triton/vLLM cache from OCI image")
	cmd.Flags().BoolVar(snapshotFlag, "snapshot", false, "Write a recursive cache directory snapshot")
	cmd.Flags().BoolVar(deltaFromSnapshotFlag, "delta-from-snapshot", false, "Create an image from cache directories added after the snapshot")

	// Information commands
	cmd.Flags().BoolVar(gpuInfoFlag, "gpu-info", false, "Display GPU-specific information")
	cmd.Flags().BoolVar(checkCompatFlag, "check-compat", false, "Check GPU compatibility with specified image")

	// Configuration options
	cmd.Flags().StringVarP(logLevel, "log-level", "l", "info", "Set logging verbosity (debug, info, warning, error)")
	cmd.Flags().BoolVarP(baremetalFlag, "baremetal", "b", false, "Enable detailed baremetal preflight checks")
	cmd.Flags().BoolVar(noGPUFlag, "no-gpu", false, "Disable GPU detection and preflight checks (for testing)")
	cmd.Flags().BoolVar(stubFlag, "stub", false, "Use mock/stub data for hardware info (for testing)")
	cmd.Flags().StringVar(builder, "builder", "", "Builder: buildah, docker, or oci (creates and pushes directly to the registry)")
	cmd.Flags().StringVar(resultPath, "result", "", "Write OCI create result as JSON to this file")
	cmd.Flags().StringVar(snapshotPath, "snapshot-file", cachesnapshot.DefaultPath, "Cache snapshot file path")
	cmd.Flags().StringArrayVar(excludedDirectories, "exclude-dir", nil, "Directory below --dir to exclude from snapshot (repeatable)")
	cmd.Flags().IntVarP(timeout, "timeout", "t", 10, "Timeout in minutes for hardware detection operations (0 = disable timeout)")

	// Mark mutually exclusive flags
	cmd.MarkFlagsMutuallyExclusive("create", "extract", "snapshot")
	cmd.MarkFlagsMutuallyExclusive("no-gpu", "gpu-info")
	cmd.MarkFlagsMutuallyExclusive("no-gpu", "check-compat")
}

func handleRunCommand(imageName, cacheDirName, logLevel, builder, resultPath, snapshotPath string, excludedDirectories []string, createFlag, extractFlag, snapshotFlag, deltaFromSnapshotFlag, baremetalFlag, noGPUFlag, checkCompatFlag, gpuInfoFlag, stubFlag bool, timeout int) {
	// Validate flag combinations
	if err := validateFlagCombinations(createFlag, extractFlag, snapshotFlag, gpuInfoFlag, checkCompatFlag, imageName, cacheDirName, stubFlag); err != nil {
		logging.Error(err)
		os.Exit(exitLogError)
	}
	if err := validateDeltaFlag(deltaFromSnapshotFlag, createFlag, builder); err != nil {
		logging.Error(err)
		os.Exit(exitDeltaError)
	}
	if err := validateExcludedDirectories(excludedDirectories, snapshotFlag); err != nil {
		logging.Error(err)
		os.Exit(exitSnapshotError)
	}

	// Configure flags before any operations so --no-gpu works with --create
	configureBoolFlags(baremetalFlag, noGPUFlag, stubFlag)

	if createFlag {
		runCreate(imageName, cacheDirName, builder, resultPath, snapshotPath, deltaFromSnapshotFlag)
		return
	}
	if snapshotFlag {
		runSnapshot(cacheDirName, snapshotPath, excludedDirectories)
		return
	}

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

func validateExcludedDirectories(excludedDirectories []string, snapshotFlag bool) error {
	if len(excludedDirectories) > 0 && !snapshotFlag {
		return errors.New("--exclude-dir can only be used with --snapshot")
	}
	return nil
}

func validateDeltaFlag(deltaFromSnapshotFlag, createFlag bool, builder string) error {
	if !deltaFromSnapshotFlag {
		return nil
	}
	if !createFlag {
		return errors.New("--delta-from-snapshot can only be used with --create")
	}
	if builder != imgbuild.OCI {
		return errors.New("--delta-from-snapshot requires --builder oci")
	}
	return nil
}

func validateFlagCombinations(createFlag, extractFlag, snapshotFlag, gpuInfoFlag, checkCompatFlag bool, imageName, cacheDirName string, stubFlag bool) error {
	actionCount := 0
	if createFlag {
		actionCount++
	}
	if extractFlag {
		actionCount++
	}
	if snapshotFlag {
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
	if (createFlag || snapshotFlag) && cacheDirName == "" {
		return errors.New("--dir is required when using --create or --snapshot")
	}

	// Stub flag validation
	if stubFlag && !gpuInfoFlag {
		return errors.New("--stub can only be used with --gpu-info")
	}

	return nil
}

func runSnapshot(cacheDir, snapshotPath string, excludedDirectories []string) {
	document, err := cachesnapshot.CaptureRoots(snapshotRootOptions(cacheDir, excludedDirectories))
	if err != nil {
		logging.Errorf("Failed to capture cache directory snapshot: %v", err)
		os.Exit(exitSnapshotError)
	}
	if err := cachesnapshot.Write(snapshotPath, document); err != nil {
		logging.Errorf("Failed to write cache directory snapshot: %v", err)
		os.Exit(exitSnapshotError)
	}
	logging.Infof("Cache directory snapshot written to %s", snapshotPath)
}

func snapshotRootOptions(cacheDir string, excludedDirectories []string) []cachesnapshot.RootOptions {
	merged := append(cachesnapshot.DefaultExcludedDirectories(), excludedDirectories...)
	seen := make(map[string]struct{}, len(merged))
	unique := make([]string, 0, len(merged))
	for _, directory := range merged {
		if _, exists := seen[directory]; exists {
			continue
		}
		seen[directory] = struct{}{}
		unique = append(unique, directory)
	}
	return []cachesnapshot.RootOptions{{
		Source:              cacheDir,
		ExcludedDirectories: unique,
	}}
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

func runCreate(imageName, cacheDir, builder, resultPath, snapshotPath string, deltaFromSnapshot bool) {
	// Chroot isolation is used in restricted container environments where creating
	// a user namespace is not available.
	if builder != imgbuild.OCI && !strings.EqualFold(os.Getenv("BUILDAH_ISOLATION"), "chroot") {
		unshare.MaybeReexecUsingUserNamespace(false)
	}

	// Check if the cache directory exists
	exists, err := utils.FilePathExists(cacheDir)
	if err != nil {
		logging.Errorf("Error checking cache file path: %v", err)
		os.Exit(exitCreateError)
	}
	if !exists {
		logging.Errorf("Cache directory does not exist: %s", cacheDir)
		os.Exit(exitCreateError)
	}

	// Initialize the image builder
	var builderInstance imgbuild.ImageBuilder
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
	if deltaFromSnapshot {
		runDeltaCreate(builderInstance, imageName, cacheDir, snapshotPath, resultPath)
		return
	}

	if resultPath == "" {
		if err := builderInstance.CreateImage(imageName, cacheDir); err != nil {
			logging.Errorf("Failed to create the OCI image: %v", err)
			os.Exit(exitCreateError)
		}
		logging.Info("OCI image created successfully.")
		return
	}

	resultBuilder, ok := builderInstance.(imgbuild.ResultImageBuilder)
	if !ok {
		logging.Errorf("Builder %q does not support structured results", builder)
		os.Exit(exitCreateError)
	}
	result, err := resultBuilder.CreateImageWithResult(imageName, cacheDir)
	if err != nil {
		logging.Errorf("Failed to create the OCI image: %v", err)
		os.Exit(exitCreateError)
	}
	if err := writeCreateResult(resultPath, result); err != nil {
		logging.Errorf("Failed to write OCI create result: %v", err)
		os.Exit(exitCreateError)
	}
	logging.Info("OCI image created successfully.")
}

func runDeltaCreate(builderInstance imgbuild.ImageBuilder, imageName, cacheDir, snapshotPath, resultPath string) {
	deltaBuilder, ok := builderInstance.(imgbuild.DeltaImageBuilder)
	if !ok {
		logging.Error("Selected builder does not support delta OCI creation")
		os.Exit(exitDeltaError)
	}
	result, err := deltaBuilder.CreateDeltaImageWithResult(imageName, cacheDir, snapshotPath)
	if err != nil {
		logging.Errorf("Failed to create OCI cache image from snapshot delta: %v", err)
		os.Exit(exitDeltaError)
	}
	if resultPath != "" {
		if err := writeCreateResult(resultPath, result); err != nil {
			logging.Errorf("Failed to write OCI create result: %v", err)
			os.Exit(exitDeltaError)
		}
	}
	if result.State == imgbuild.CreateStateUnchanged {
		logging.Info("No new cache directories found after exclusions; OCI image creation skipped.")
		return
	}
	logging.Info("OCI cache image created successfully.")
}

func writeCreateResult(path string, result *imgbuild.CreateResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".mcv-result-")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(result); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
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
