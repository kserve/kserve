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

	"github.com/jaypipes/ghw"
	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/kernelcache/mcv/pkg/config"
)

func GetProductName(id int) (name string, err error) {
	xpus, errAcc := ghw.Accelerator()
	if errAcc != nil {
		logging.Error("failed to get accelerator info:", errAcc)
	} else {
		for i, device := range xpus.Devices {
			if i == id && device.PCIDevice != nil {
				return device.PCIDevice.Product.Name, nil
			}
		}
	}
	return "", errors.New("PCI device information unavailable")
}

// DetectAccelerators detects hardware accelerators and enables GPU logic if supported hardware is found.
// If stub mode is enabled, it simulates the accelerator selected by MCV_STUB_PROFILE (default: amd).
// If no hardware accelerators are found, it returns nil without an error.
func DetectAccelerators() (accInfo *ghw.AcceleratorInfo) {
	if config.IsStubEnabled() {
		logging.Debug("Stub mode configured, simulating accelerator device")
		return stubbedAcceleratorInfo(activeStubProfile())
	}

	acc, err := ghw.Accelerator()
	if err != nil {
		logging.Debugf("no Accelerator detected")
		return nil
	}
	return acc
}
