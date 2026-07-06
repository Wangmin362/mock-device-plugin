/*
Copyright 2025 The HAMi Authors.

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

// Package mthreads implements a *mock* device plugin for Moore Threads
// (摩尔线程) GPUs. Unlike the other vendors in this repo, the Moore Threads
// support in HAMi does NOT rely on a `hami.io/node-*-register` annotation:
// HAMi's pkg/device/mthreads reads the node's Capacity directly. Therefore the
// simplest and most robust way to fake Moore Threads cards is to advertise the
// three extended resources straight from config, independent of any node
// annotation:
//
//	mthreads.com/vgpu         = <card count>
//	mthreads.com/sgpu-memory  = <card count> * <memory units per card>   (unit = 512MiB, 96 => 48GiB)
//	mthreads.com/sgpu-core    = <card count> * <core units per card>     (16 per card)
//
// This mirrors what a real Moore Threads device-plugin would put into
// Node.Status.Capacity, so both the HAMi scheduler and vast can discover the
// cards. Allocate is a no-op (inherited from the mock server) — this is for
// demonstration/onboarding only, there is no real device behind it.
package mthreads

import (
	"strconv"

	"github.com/HAMi/mock-device-plugin/internal/pkg/api/device"
	"github.com/HAMi/mock-device-plugin/internal/pkg/mock"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const (
	// MthreadsGPUDevice is the device Type / vendor common word, kept in sync
	// with HAMi's pkg/device/mthreads (MthreadsGPUDevice / MthreadsGPUCommonWord).
	MthreadsGPUDevice     = "Mthreads"
	MthreadsGPUCommonWord = "Mthreads"

	// Per-card unit conventions, matching HAMi's mthreads constants
	// (coresPerMthreadsGPU = 16, memoryPerMthreadsGPU = 96, memory unit = 512MiB).
	defaultMemoryUnitPerGPU = 96 // 96 * 512MiB = 49152MiB = 48GiB (MTT S4000)
	defaultCoreUnitPerGPU   = 16
	defaultDeviceType       = "MTT S4000"
)

// MthreadsConfig is the yaml config for the mock Moore Threads device.
// The three resource*Name fields keep the same shape as the real HAMi device
// config so the plugin can share the hami-scheduler-device ConfigMap layout;
// the device* fields are mock-only knobs describing the fake cards to report.
type MthreadsConfig struct {
	ResourceCountName  string `yaml:"resourceCountName"`
	ResourceMemoryName string `yaml:"resourceMemoryName"`
	ResourceCoreName   string `yaml:"resourceCoreName"`

	// Mock-only knobs.
	DeviceCount      int    `yaml:"deviceCount"`      // number of fake cards to advertise on the node
	DeviceMemoryUnit int    `yaml:"deviceMemoryUnit"` // 512MiB units per card (96 => 48GiB)
	DeviceCoreUnit   int    `yaml:"deviceCoreUnit"`   // sgpu-core units per card (16)
	DeviceType       string `yaml:"deviceType"`       // model string, e.g. "MTT S4000"
}

type MthreadsDevices struct {
	config MthreadsConfig
}

// InitMthreadsDevice builds the mock device. It returns nil (i.e. the vendor is
// not activated) when the config section is absent/empty, so mounting a shared
// device-config that lacks a mthreads section never spins up a bogus plugin.
func InitMthreadsDevice(config MthreadsConfig) *MthreadsDevices {
	if config.ResourceMemoryName == "" || config.ResourceCoreName == "" || config.ResourceCountName == "" {
		return nil
	}
	if config.DeviceCount <= 0 {
		klog.InfoS("mthreads mock device configured but deviceCount<=0, not activating")
		return nil
	}
	if config.DeviceMemoryUnit <= 0 {
		config.DeviceMemoryUnit = defaultMemoryUnitPerGPU
	}
	if config.DeviceCoreUnit <= 0 {
		config.DeviceCoreUnit = defaultCoreUnitPerGPU
	}
	if config.DeviceType == "" {
		config.DeviceType = defaultDeviceType
	}
	klog.InfoS("initializing mthreads mock device",
		"count", config.DeviceCount,
		"memoryUnitPerGPU", config.DeviceMemoryUnit,
		"coreUnitPerGPU", config.DeviceCoreUnit,
		"deviceType", config.DeviceType,
		"resourceCount", config.ResourceCountName,
		"resourceMemory", config.ResourceMemoryName,
		"resourceCore", config.ResourceCoreName,
	)
	return &MthreadsDevices{config: config}
}

func (dev *MthreadsDevices) CommonWord() string {
	return MthreadsGPUCommonWord
}

// GetNodeDevices synthesizes the per-card DeviceInfo list purely from config
// (node-independent). It is primarily used for logging/completeness; the actual
// reporting to kubelet is driven by GetResource.
func (dev *MthreadsDevices) GetNodeDevices(n *corev1.Node) ([]*device.DeviceInfo, error) {
	devs := make([]*device.DeviceInfo, 0, dev.config.DeviceCount)
	for i := 0; i < dev.config.DeviceCount; i++ {
		devs = append(devs, &device.DeviceInfo{
			ID:           n.Name + "-mthreads-" + strconv.Itoa(i),
			Index:        uint(i),
			Count:        100, // time-slice shares, matching HAMi DeviceInfo.Count
			Devmem:       int32(dev.config.DeviceMemoryUnit),
			Devcore:      int32(dev.config.DeviceCoreUnit),
			Type:         dev.config.DeviceType,
			Health:       true,
			DeviceVendor: MthreadsGPUCommonWord,
		})
	}
	return devs, nil
}

// GetResource advertises the three Moore Threads extended resources on the node
// straight from config, independent of any node annotation. Reporting the count
// resource ourselves (as opposed to relying on CheckHealthy) is required because
// nothing else provides `mthreads.com/vgpu` on a mock node.
func (dev *MthreadsDevices) GetResource(n *corev1.Node) map[string]int {
	countName := device.GetResourceName(dev.config.ResourceCountName)
	memoryName := device.GetResourceName(dev.config.ResourceMemoryName)
	coreName := device.GetResourceName(dev.config.ResourceCoreName)

	resourceMap := map[string]int{
		countName:  dev.config.DeviceCount,
		memoryName: dev.config.DeviceCount * dev.config.DeviceMemoryUnit,
		coreName:   dev.config.DeviceCount * dev.config.DeviceCoreUnit,
	}
	klog.InfoS("Add mthreads resources",
		countName, resourceMap[countName],
		memoryName, resourceMap[memoryName],
		coreName, resourceMap[coreName],
	)
	return resourceMap
}

func (dev *MthreadsDevices) RunManager() {
	// All three resources share the vendor namespace (mthreads.com).
	lmock := mock.NewMockLister(device.GetVendorName(dev.config.ResourceMemoryName))
	go device.Register(lmock, dev)
	mockmanager := dpm.NewManager(lmock)
	klog.Infof("Running mocking dp: %s", dev.CommonWord())
	mockmanager.Run()
}
