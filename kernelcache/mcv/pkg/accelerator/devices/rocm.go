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

package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/kserve/kserve/mcv/pkg/config"
	"github.com/kserve/kserve/mcv/pkg/utils"

	logging "github.com/sirupsen/logrus"
)

const rocmHwType = config.GPU

var (
	rocmAccImpl = gpuROCm{}
	rocmType    DeviceType
)

type gpuROCm struct {
	devices    map[int]GPUDevice // GPU identifiers mapped to device info
	name       string
	deviceType DeviceType
	hwType     string
	tritonInfo []TritonGPUInfo
	summaries  []DeviceSummary
}

// SetName sets the name of the ROCM device.
func (d *gpuROCm) SetName(name string) {
	d.name = name
}

// SetDeviceType sets the device type of the ROCM device.
func (d *gpuROCm) SetDeviceType(deviceType DeviceType) {
	d.deviceType = deviceType
}

// SetHwType sets the hardware type of the ROCM device.
func (d *gpuROCm) SetHwType(hwType string) {
	d.hwType = hwType
}

// SetTritonInfo sets the Triton GPU information for the ROCM device.
// When restoring from cache, this also populates the devices map.
func (d *gpuROCm) SetTritonInfo(info []TritonGPUInfo) {
	d.tritonInfo = info

	// Reinitialize to prevent stale GPUs from previous state
	d.devices = make(map[int]GPUDevice, len(info))
	for _, tritonInfo := range info {
		d.devices[tritonInfo.ID] = GPUDevice{
			ID:         tritonInfo.ID,
			TritonInfo: tritonInfo,
			// Summary will be set by SetSummaries
		}
	}
}

// SetSummaries sets the summaries for the ROCM device.
// When restoring from cache, this also updates the Summary field in devices map.
func (d *gpuROCm) SetSummaries(summaries []DeviceSummary) {
	d.summaries = summaries

	// Update Summary in devices map if it exists
	if d.devices != nil {
		for _, summary := range summaries {
			// Parse GPU ID from summary.ID (which is a string like "0", "1", etc.)
			var gpuID int
			if _, err := fmt.Sscanf(summary.ID, "%d", &gpuID); err == nil {
				if dev, exists := d.devices[gpuID]; exists {
					dev.Summary = summary
					d.devices[gpuID] = dev
				}
			}
		}
	}
}

type ROCMGPUInfo struct {
	GPUInfo map[int]*ROCMCardInfo
	DrvInfo *ROCMSystemInfo
}

type ROCMCardInfo struct {
	UniqueID           string `json:"Unique ID"`
	SerialNumber       string `json:"Serial Number"`
	VRAMTotalMemory    string `json:"VRAM Total Memory (B)"`
	VRAMUsedMemory     string `json:"VRAM Total Used Memory (B)"`
	VISVRAMTotalMemory string `json:"VIS_VRAM Total Memory (B)"`
	VISVRAMUsedMemory  string `json:"VIS_VRAM Total Used Memory (B)"`
	GTTTotalMemory     string `json:"GTT Total Memory (B)"`
	GTTUsedMemory      string `json:"GTT Used Memory (B)"`
	CardSeries         string `json:"Card Series"`
	CardModel          string `json:"Card Model"`
	CardVendor         string `json:"Card Vendor"`
	CardSKU            string `json:"Card SKU"`
	SubsystemID        string `json:"Subsystem ID"`
	DeviceRev          string `json:"Device Rev"`
	NodeID             string `json:"Node ID"`
	GUID               string `json:"GUID"`
	GFXVersion         string `json:"GFX Version"`
}

type ROCMSystemInfo struct {
	System struct {
		DriverVersion string `json:"Driver version"`
	} `json:"system"`
}

func rocmCheck(r *Registry) {
	if err := initROCmLib(); err != nil {
		logging.Debugf("Error initializing ROCm: %v", err)
		return
	}
	rocmType = ROCM
	if err := addDeviceInterface(r, rocmType, rocmHwType, rocmDeviceStartup); err == nil {
		logging.Debugf("Using %s to obtain GPU info", rocmAccImpl.Name())
	} else {
		logging.Debugf("Error registering rocm-smi: %v", err)
	}
}

func rocmDeviceStartup() Device {
	a := rocmAccImpl
	if err := a.InitLib(); err != nil {
		logging.Errorf("Error initializing %s: %v", rocmType.String(), err)
		return nil
	}
	if err := a.Init(); err != nil {
		logging.Errorf("Failed to init device: %v", err)
		return nil
	}
	logging.Debugf("Using %s to obtain GPU info", rocmType.String())
	logging.Debugf("ROCm device startup completed")
	return &a
}

func initROCmLib() error {
	if utils.HasApp("rocm-smi") {
		return nil
	}
	return errors.New("couldn't find rocm-smi")
}

func (r *gpuROCm) InitLib() error {
	return initROCmLib()
}

func (r *gpuROCm) Name() string {
	return rocmType.String()
}

func (r *gpuROCm) DevType() DeviceType {
	return rocmType
}

func (r *gpuROCm) HwType() string {
	return rocmHwType
}

// Init initializes and starts the GPU info collection using a **single `rocm-smi` command**
func (r *gpuROCm) Init() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gpuInfoList, err := getAllROCmGPUInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get GPU information: %w", err)
	}

	// Populate the devices map
	r.devices = make(map[int]GPUDevice, len(gpuInfoList.GPUInfo))
	for gpuID, info := range gpuInfoList.GPUInfo {
		memTotal, _ := strconv.ParseUint(info.VRAMTotalMemory, 10, 64)
		name := "card" + strconv.Itoa(gpuID)
		prodName, _ := GetProductName(gpuID) // TODO error checking in the future
		r.devices[gpuID] = GPUDevice{
			ID: gpuID,
			TritonInfo: TritonGPUInfo{
				Name:              name,
				UUID:              info.UniqueID,
				ComputeCapability: "",
				Arch:              info.GFXVersion,
				WarpSize:          64,
				MemoryTotalMB:     memTotal / (1024 * 1024),
				Backend:           hipBackend,
				ID:                gpuID,
			},
			Summary: DeviceSummary{
				ID:            strconv.Itoa(gpuID),
				ProductName:   prodName,
				DriverVersion: gpuInfoList.DrvInfo.System.DriverVersion,
			},
		}
	}

	return nil
}

// Shutdown stops the GPU metric collector
func (r *gpuROCm) Shutdown() bool {
	return true
}

func getAllROCmGPUInfo(ctx context.Context) (*ROCMGPUInfo, error) {
	gpus, err := getROCmGPUInfo(ctx)
	if err != nil {
		return nil, errors.New("could not get GPU info")
	}
	system, err := getROCmSystemInfo(ctx)
	if err != nil {
		return nil, errors.New("could not get system info")
	}

	return &ROCMGPUInfo{
		GPUInfo: gpus,
		DrvInfo: system,
	}, nil
}

// Fetches all GPUs' info in **one single rocm-smi call**
func getROCmGPUInfo(ctx context.Context) (map[int]*ROCMCardInfo, error) {
	cmd := exec.CommandContext(ctx, "rocm-smi", "--json", "--showproductname", "--showuniqueid", "--showserial", "--showmeminfo", "all")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute rocm-smi: %w", err)
	}

	var gpuInfo map[string]*ROCMCardInfo
	if err = json.Unmarshal(output, &gpuInfo); err != nil {
		return nil, fmt.Errorf("failed to parse rocm-smi output: %w", err)
	}

	prettyJSON, err := json.MarshalIndent(gpuInfo, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to pretty print JSON: %w", err)
	}

	logging.Debugf("ROCM JSON output:\n%s", string(prettyJSON))

	// Convert map keys from "GPUX" to int keys
	parsedGPUs := make(map[int]*ROCMCardInfo)
	for key, gpu := range gpuInfo {
		var gpuID int
		_, err := fmt.Sscanf(key, "card%d", &gpuID)
		if err == nil {
			parsedGPUs[gpuID] = gpu
		}
	}

	return parsedGPUs, nil
}

// Fetches all GPUs' info in **one single rocm-smi call**
func getROCmSystemInfo(ctx context.Context) (*ROCMSystemInfo, error) {
	cmd := exec.CommandContext(ctx, "rocm-smi", "--json", "--showdriverversion")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute rocm-smi: %w", err)
	}

	var systemInfo ROCMSystemInfo
	if err = json.Unmarshal(output, &systemInfo); err != nil {
		return nil, fmt.Errorf("failed to parse rocm-smi output: %w", err)
	}

	prettyJSON, err := json.MarshalIndent(systemInfo, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to pretty print JSON: %w", err)
	}

	logging.Debugf("ROCM JSON output:\n%s", string(prettyJSON))

	return &systemInfo, nil
}

// GetAllGPUInfo returns a list of GPU info for all devices
func (r *gpuROCm) GetAllGPUInfo() ([]TritonGPUInfo, error) {
	allTritonInfo := make([]TritonGPUInfo, 0, len(r.devices))
	for gpuID := range r.devices {
		dev := r.devices[gpuID]
		allTritonInfo = append(allTritonInfo, dev.TritonInfo)
		logging.Debugf("GPU %d: %+v", gpuID, dev.TritonInfo)
	}
	return allTritonInfo, nil
}

// GetGPUInfo retrieves the stored GPU info for a specific device ID.
func (r *gpuROCm) GetGPUInfo(gpuID int) (TritonGPUInfo, error) {
	dev, exists := r.devices[gpuID]
	if !exists {
		return TritonGPUInfo{}, fmt.Errorf("GPU device %d not found", gpuID)
	}
	return dev.TritonInfo, nil
}

func (r *gpuROCm) GetAllSummaries() ([]DeviceSummary, error) {
	// Check if summaries are already cached
	if len(r.summaries) > 0 {
		logging.Debugf("Returning cached summaries for ROCM device %s", r.Name())
		return r.summaries, nil
	}

	// Fallback to default behavior if cache is unavailable
	var allAccInfo []DeviceSummary
	for gpuID := range r.devices {
		dev := r.devices[gpuID]
		allAccInfo = append(allAccInfo, dev.Summary)
		logging.Debugf("GPU %d: %+v", gpuID, dev.Summary)
	}
	r.summaries = allAccInfo // Cache the summaries for future calls
	return allAccInfo, nil
}

func (r *gpuROCm) GetSummary(gpuID int) (DeviceSummary, error) {
	dev, exists := r.devices[gpuID]
	if !exists {
		return DeviceSummary{}, fmt.Errorf("GPU device %d not found", gpuID)
	}
	return dev.Summary, nil
}
