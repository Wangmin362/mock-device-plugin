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

package mthreads

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func fullConfig() MthreadsConfig {
	return MthreadsConfig{
		ResourceCountName:  "mthreads.com/vgpu",
		ResourceMemoryName: "mthreads.com/sgpu-memory",
		ResourceCoreName:   "mthreads.com/sgpu-core",
		DeviceCount:        4,
		DeviceMemoryUnit:   96,
		DeviceCoreUnit:     16,
		DeviceType:         "MTT S4000",
	}
}

func TestInitMthreadsDevice_Guards(t *testing.T) {
	// Missing resource names -> not activated.
	if InitMthreadsDevice(MthreadsConfig{DeviceCount: 4}) != nil {
		t.Fatal("expected nil when resource names are empty")
	}
	// deviceCount <= 0 -> not activated.
	cfg := fullConfig()
	cfg.DeviceCount = 0
	if InitMthreadsDevice(cfg) != nil {
		t.Fatal("expected nil when deviceCount<=0")
	}
	// Fully configured -> activated.
	if InitMthreadsDevice(fullConfig()) == nil {
		t.Fatal("expected non-nil for a fully configured mthreads device")
	}
}

func TestInitMthreadsDevice_Defaults(t *testing.T) {
	cfg := MthreadsConfig{
		ResourceCountName:  "mthreads.com/vgpu",
		ResourceMemoryName: "mthreads.com/sgpu-memory",
		ResourceCoreName:   "mthreads.com/sgpu-core",
		DeviceCount:        1,
	}
	dev := InitMthreadsDevice(cfg)
	if dev == nil {
		t.Fatal("expected non-nil device")
	}
	if dev.config.DeviceMemoryUnit != defaultMemoryUnitPerGPU {
		t.Fatalf("memory unit default = %d, want %d", dev.config.DeviceMemoryUnit, defaultMemoryUnitPerGPU)
	}
	if dev.config.DeviceCoreUnit != defaultCoreUnitPerGPU {
		t.Fatalf("core unit default = %d, want %d", dev.config.DeviceCoreUnit, defaultCoreUnitPerGPU)
	}
	if dev.config.DeviceType != defaultDeviceType {
		t.Fatalf("device type default = %q, want %q", dev.config.DeviceType, defaultDeviceType)
	}
}

func TestGetResource(t *testing.T) {
	dev := InitMthreadsDevice(fullConfig())
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "hw-4090d-50"}}
	res := dev.GetResource(node)

	if got := res["vgpu"]; got != 4 {
		t.Errorf("vgpu = %d, want 4", got)
	}
	if got := res["sgpu-memory"]; got != 4*96 {
		t.Errorf("sgpu-memory = %d, want %d", got, 4*96)
	}
	if got := res["sgpu-core"]; got != 4*16 {
		t.Errorf("sgpu-core = %d, want %d", got, 4*16)
	}
}

func TestGetNodeDevices(t *testing.T) {
	dev := InitMthreadsDevice(fullConfig())
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "hw-4090d-50"}}
	devs, err := dev.GetNodeDevices(node)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 4 {
		t.Fatalf("device count = %d, want 4", len(devs))
	}
	first := devs[0]
	if first.ID != "hw-4090d-50-mthreads-0" {
		t.Errorf("id = %q, want hw-4090d-50-mthreads-0", first.ID)
	}
	if first.Devmem != 96 || first.Devcore != 16 {
		t.Errorf("devmem/devcore = %d/%d, want 96/16", first.Devmem, first.Devcore)
	}
	if first.Type != "MTT S4000" || first.DeviceVendor != MthreadsGPUCommonWord {
		t.Errorf("type/vendor = %q/%q", first.Type, first.DeviceVendor)
	}
}
