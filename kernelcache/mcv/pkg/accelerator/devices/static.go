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
	"errors"

	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/mcv/pkg/config"
)

type StaticDevice struct {
	name       string
	deviceType DeviceType
	hwType     string
	tritonInfo []TritonGPUInfo
	summaries  []DeviceSummary
}

func (d *StaticDevice) Name() string        { return d.name }
func (d *StaticDevice) DevType() DeviceType { return d.deviceType }
func (d *StaticDevice) HwType() string      { return d.hwType }
func (d *StaticDevice) InitLib() error      { return nil }
func (d *StaticDevice) Init() error         { return nil }
func (d *StaticDevice) Shutdown() bool      { return true }
func (d *StaticDevice) GetGPUInfo(gpuID int) (TritonGPUInfo, error) {
	if gpuID < 0 || gpuID >= len(d.tritonInfo) {
		return TritonGPUInfo{}, errors.New("invalid GPU ID")
	}
	return d.tritonInfo[gpuID], nil
}

func (d *StaticDevice) GetSummary(gpuID int) (DeviceSummary, error) {
	if gpuID < 0 || gpuID >= len(d.summaries) {
		return DeviceSummary{}, errors.New("invalid GPU ID")
	}
	return d.summaries[gpuID], nil
}

func (d *StaticDevice) GetAllGPUInfo() ([]TritonGPUInfo, error) {
	return d.tritonInfo, nil
}

func (d *StaticDevice) GetAllSummaries() ([]DeviceSummary, error) {
	return d.summaries, nil
}

// staticCheck registers static devices when stub mode is enabled
func staticCheck(r *Registry) {
	logging.Debugf("Registering static device for stub mode")
	if err := addDeviceInterface(r, 1, config.GPU, staticDeviceStartup); err == nil {
		logging.Debugf("Using static device to obtain GPU info")
	} else {
		logging.Debugf("Error registering static device: %v", err)
	}
}

func staticDeviceStartup() Device {
	cache := NewStubbedDeviceCache()
	convertedDevices := make(map[string]Device)
	for key, cachedDevice := range cache.Devices {
		convertedDevices[key] = &StaticDevice{
			name:       cachedDevice.Name,
			deviceType: cachedDevice.DeviceType,
			hwType:     cachedDevice.HwType,
			tritonInfo: cachedDevice.TritonInfo,
			summaries:  cachedDevice.Summaries,
		}
	}
	saveCache(convertedDevices) // Call saveCache to persist the cache
	// Use the first device from the stubbed cache
	for _, cachedDevice := range cache.Devices {
		return &StaticDevice{
			name:       cachedDevice.Name,
			deviceType: cachedDevice.DeviceType,
			hwType:     cachedDevice.HwType,
			tritonInfo: cachedDevice.TritonInfo,
			summaries:  cachedDevice.Summaries,
		}
	}
	return nil
}

func NewStubbedDeviceCache() *DeviceCache {
	return &DeviceCache{
		Devices: map[string]CachedDevice{
			"gpu": {
				Name:       stubbedAMDName,
				DeviceType: 1, // DeviceType for GPU, adjust if you have a constant
				HwType:     "gpu",
				TritonInfo: []TritonGPUInfo{
					{
						Name:              "card0",
						UUID:              "daff740f-0000-1000-8062-0165038984ec",
						ComputeCapability: "",
						Arch:              gfxArchMI210,
						WarpSize:          64,
						MemoryTotalMB:     65520,
						PTXVersion:        0,
						Backend:           hipBackend,
						ID:                0,
					},
					{
						Name:              "card1",
						UUID:              "acff740f-0000-1000-806b-c6ef57f28db1",
						ComputeCapability: "",
						Arch:              gfxArchMI210,
						WarpSize:          64,
						MemoryTotalMB:     65520,
						PTXVersion:        0,
						Backend:           hipBackend,
						ID:                1,
					},
				},
				Summaries: []DeviceSummary{
					{
						ID:            "0",
						DriverVersion: "6.12.10-100.fc40.x86_64",
						ProductName:   "STUBBED Aldebaran/MI200 [Instinct MI210]",
					},
					{
						ID:            "1",
						DriverVersion: "6.12.10-100.fc40.x86_64",
						ProductName:   "STUBBED Aldebaran/MI200 [Instinct MI210]",
					},
				},
			},
		},
	}
}
