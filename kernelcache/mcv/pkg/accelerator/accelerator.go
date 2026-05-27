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

package accelerator

//nolint:gci // The supported device imports are kept separate.
import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/pkg/errors"
	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/mcv/pkg/accelerator/devices"
	"github.com/kserve/kserve/mcv/pkg/config"
)

var (
	globalRegistry *AcceleratorRegistry
	once           sync.Once
)

// Accelerator represents an implementation of... equivalent Accelerator device.
type Accelerator interface {
	// Device returns an underlying accelerator device implementation...
	Device() devices.Device
	// IsRunning returns whether or not that device is running
	IsRunning() bool
	// stop stops an accelerator and unregisters it
	stop()
}

type accelerator struct {
	dev     devices.Device // Device Accelerator Interface
	running bool
}

type AcceleratorRegistry struct {
	Accelerators map[string]Accelerator
}

// GetAcceleratorRegistry gets the default AcceleratorRegistry instance
func GetAcceleratorRegistry() *AcceleratorRegistry {
	once.Do(func() {
		globalRegistry = &AcceleratorRegistry{
			Accelerators: map[string]Accelerator{},
		}
	})
	return globalRegistry
}

// SetAcceleratorRegistry replaces the global registry instance
// NOTE: All plugins will need to be manually registered
// after this function is called.
func SetAcceleratorRegistry(registry *AcceleratorRegistry) {
	globalRegistry = registry
}

// RegisterAccelerator adds an accelerator to the registry
func (r *AcceleratorRegistry) RegisterAccelerator(a Accelerator) {
	if a == nil || a.Device() == nil {
		logging.Errorf("Cannot register a nil accelerator or device")
		return
	}
	_, ok := r.Accelerators[a.Device().HwType()]
	if ok {
		logging.Debugf("Accelerator with type %s already exists", a.Device().HwType())
		return
	}
	r.Accelerators[a.Device().HwType()] = a
}

// UnregisterAccelerator removes an accelerator from the registry
func (r *AcceleratorRegistry) UnregisterAccelerator(a Accelerator) bool {
	_, exists := r.Accelerators[a.Device().HwType()]
	if exists {
		delete(r.Accelerators, a.Device().HwType())
		return true
	}
	logging.Errorf("Accelerator with type %s doesn't exist", a.Device().HwType())
	return false
}

// GetActiveAccelerators returns all active accelerators
func (r *AcceleratorRegistry) GetActiveAccelerators() map[string]Accelerator {
	acc := map[string]Accelerator{}

	if len(r.Accelerators) == 0 {
		// No accelerators found
		return nil
	}

	for _, a := range r.Accelerators {
		if a.IsRunning() {
			d := a.Device()
			acc[d.HwType()] = a
		}
	}

	return acc
}

// GetActiveAcceleratorByType returns an active accelerator by type
func (r *AcceleratorRegistry) GetActiveAcceleratorByType(t string) Accelerator {
	if len(r.Accelerators) == 0 {
		// No accelerators found
		logging.Infof("No accelerators found")
		return nil
	}

	for _, a := range r.Accelerators {
		if a.Device().HwType() == t && a.IsRunning() {
			return a
		}
	}
	return nil
}

func New(atype string, sleep bool) (Accelerator, error) {
	var d devices.Device
	maxDeviceInitRetry := 10
	// Init the available devices.
	logging.Debugf("Starting up device of type %s", atype)
	r := devices.GetRegistry()
	devs := r.GetAllDeviceTypes()
	numDevs := len(devs)
	if numDevs == 0 || !slices.Contains(devs, atype) {
		return nil, errors.New("no devices found")
	}
	logging.Debugf("Found %d device(s): %v", numDevs, devs)
	logging.Debugf("Initializing the Accelerator of type %v", atype)

	for i := 0; i < maxDeviceInitRetry; i++ {
		if d = devices.Startup(atype, r); d == nil {
			logging.Errorf("Could not init the %s device going to try again", atype)
			if sleep {
				// The GPU operators can be slow to start up, so we wait a bit before retrying.
				time.Sleep(6 * time.Second)
			}
			continue
		}
		logging.Debugf("Startup %s Accelerator successful", atype)
		break
	}

	if d == nil {
		return nil, fmt.Errorf("failed to initialize device of type %s after %d retries", atype, maxDeviceInitRetry)
	}

	return &accelerator{
		dev:     d,
		running: true,
	}, nil
}

func ShutdownAccelerators() {
	if accelerators := GetAcceleratorRegistry().GetActiveAccelerators(); accelerators != nil {
		for _, a := range accelerators {
			logging.Debugf("Shutting down %s", a.Device().DevType())
			a.stop()
		}
	} else {
		logging.Debugf("No devices to shutdown")
	}
}

// Stop shutsdown an accelerator
func (a *accelerator) stop() {
	if !a.dev.Shutdown() {
		logging.Error("error shutting down the device")
		return
	}

	if shutdown := GetAcceleratorRegistry().UnregisterAccelerator(a); !shutdown {
		logging.Error("error shutting down the accelerator")
		return
	}

	logging.Debug("Accelerator stopped")
}

// Device returns an accelerator interface
func (a *accelerator) Device() devices.Device {
	return a.dev
}

// DeviceType returns the accelerator's underlying device type
func (a *accelerator) DevType() string {
	return a.dev.DevType().String()
}

// IsRunning returns the running status of an accelerator
func (a *accelerator) IsRunning() bool {
	return a.running
}

func GetActiveAcceleratorByType(t string) Accelerator {
	return GetAcceleratorRegistry().GetActiveAcceleratorByType(t)
}

func GetAccelerators() map[string]Accelerator {
	return GetAcceleratorRegistry().GetActiveAccelerators()
}

// SummarizeGPUs provides a summary of the GPU fleet by retrieving the active GPU accelerator
// and summarizing its associated device. If no active GPU accelerator is found, an error is returned.
//
// Returns:
//   - *devices.GPUFleetSummary: A summary of the GPU fleet if an active accelerator is found.
//   - error: An error if no active accelerator is found or if summarizing the device fails.
func SummarizeGPUs() (*devices.GPUFleetSummary, error) {
	acc := GetActiveAcceleratorByType(config.GPU)
	if acc == nil {
		return nil, errors.New("no active accelerator found")
	}
	return devices.SummarizeDevice(acc.Device())
}
