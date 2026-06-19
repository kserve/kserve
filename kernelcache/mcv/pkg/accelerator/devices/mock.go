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
	"os"

	logging "github.com/sirupsen/logrus"
)

var mockDevice = MOCK

type MockDevice struct {
	mockDevice          DeviceType
	name                string
	collectionSupported bool
}

func mockCheck(r *Registry) {
	logging.Debugf("Checking for mock device support")
	if os.Getenv("MCV_ENABLE_MOCK_DEVICE") == "" {
		return
	}
	logging.Debugf("Mock device enabled via MCV_ENABLE_MOCK_DEVICE environment variable")
	// Register mock device under MOCK key
	if err := addDeviceInterface(r, mockDevice, mockDevice.String(), MockDeviceDeviceStartup); err != nil {
		logging.Debugf("couldn't register mock device %v", err)
	}
	logging.Debugf("Using %s interface to obtain Device info", mockDevice.String())
}

func MockDeviceDeviceStartup() Device {
	d := &MockDevice{
		mockDevice:          mockDevice,
		name:                mockDevice.String(),
		collectionSupported: true,
	}

	logging.Debugf("MockDevice startup completed")
	return d
}

func (d *MockDevice) Name() string {
	return d.name
}

func (d *MockDevice) DevType() DeviceType {
	return d.mockDevice
}

func (d *MockDevice) HwType() string {
	return d.mockDevice.String()
}

func (d *MockDevice) InitLib() error {
	return nil
}

func (d *MockDevice) Init() error {
	return nil
}

func (d *MockDevice) Shutdown() bool {
	GetRegistry().Unregister(d.DevType())
	return true
}

func (d *MockDevice) GetGPUInfo(gpuID int) (TritonGPUInfo, error) {
	return TritonGPUInfo{}, nil
}

func (d *MockDevice) GetAllGPUInfo() ([]TritonGPUInfo, error) {
	return []TritonGPUInfo{}, nil
}

func (d *MockDevice) GetAllSummaries() ([]DeviceSummary, error) {
	return []DeviceSummary{}, nil
}

func (d *MockDevice) GetSummary(gpuID int) (DeviceSummary, error) {
	return DeviceSummary{}, nil
}
