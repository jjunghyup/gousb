// Copyright 2013 Google Inc.  All rights reserved.
// Copyright 2016 the gousb Authors.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gousb

import (
	"fmt"
	"sort"
	"sync"
)

// Descriptor is a representation of a USB device descriptor.
type Descriptor struct {
	// Bus information
	Bus     int // The bus on which the device was detected
	Address int // The address of the device on the bus

	// Version information
	Spec   BCD // USB Specification Release Number
	Device BCD // The device version

	// Product information
	Vendor  ID // The Vendor identifer
	Product ID // The Product identifier

	// Protocol information
	Class    Class    // The class of this device
	SubClass Class    // The sub-class (within the class) of this device
	Protocol Protocol // The protocol (within the sub-class) of this device

	// Configuration information
	Configs map[int]ConfigInfo
}

// String returns a human-readable version of the device descriptor.
func (d *Descriptor) String() string {
	return fmt.Sprintf("%d.%d: %s:%s (available configs: %v)", d.Bus, d.Address, d.Vendor, d.Product, d.sortedConfigIds())
}

// Device represents an opened USB device.
type Device struct {
	handle *libusbDevHandle

	// Embed the device information for easy access
	*Descriptor

	// Claimed config
	mu      sync.Mutex
	claimed *Config
}

func (d *Descriptor) sortedConfigIds() []int {
	var cfgs []int
	for c := range d.Configs {
		cfgs = append(cfgs, c)
	}
	sort.Ints(cfgs)
	return cfgs
}

// String represents a human readable representation of the device.
func (d *Device) String() string {
	return fmt.Sprintf("vid=%s,pid=%s,bus=%d,addr=%d", d.Vendor, d.Product, d.Bus, d.Address)
}

// Reset performs a USB port reset to reinitialize a device.
func (d *Device) Reset() error {
	if d.handle == nil {
		return fmt.Errorf("Reset() called on %s after Close", d)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.claimed != nil {
		return fmt.Errorf("can't reset device %s while it has an active configuration %s", d, d.claimed)
	}
	return libusb.reset(d.handle)
}

// ActiveConfigNum returns the config id of the active configuration.
// The value corresponds to the ConfigInfo.Config field of one of the
// ConfigInfos of this Device.
func (d *Device) ActiveConfigNum() (int, error) {
	if d.handle == nil {
		return 0, fmt.Errorf("ActiveConfig() called on %s after Close", d)
	}
	ret, err := libusb.getConfig(d.handle)
	return int(ret), err
}

// Config returns a USB device set to use a particular config.
// The cfgNum provided is the config id (not the index) of the configuration to
// set, which corresponds to the ConfigInfo.Config field.
// USB supports only one active config per device at a time. Config claims the
// device before setting the desired config and keeps it locked until Close is
// called.
func (d *Device) Config(cfgNum int) (*Config, error) {
	if d.handle == nil {
		return nil, fmt.Errorf("Config(%d) called on %s after Close", cfgNum, d)
	}
	info, ok := d.Descriptor.Configs[cfgNum]
	if !ok {
		return nil, fmt.Errorf("configuration id %d not found in the descriptor of the device %s. Available config ids: %v", cfgNum, d, d.Descriptor.sortedConfigIds())
	}
	cfg := &Config{
		Info:    info,
		dev:     d,
		claimed: make(map[int]bool),
	}
	if err := libusb.setConfig(d.handle, uint8(cfgNum)); err != nil {
		return nil, fmt.Errorf("failed to set active config %d for the device %s: %v", cfgNum, d, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.claimed = cfg
	return cfg, nil
}

// Default opens interface #0 with alternate setting #0 of the currently active
// config. It's intended as a shortcut for devices that have the simplest
// interface of a single config, interface and alternate setting.
func (d *Device) Default() (*Interface, error) {
	cfgNum, err := d.ActiveConfigNum()
	if err != nil {
		return nil, fmt.Errorf("failed to get active config number of device %s: %v", d, err)
	}
	cfg, err := d.Config(cfgNum)
	if err != nil {
		return nil, fmt.Errorf("failed to claim config %d of device %s: %v", cfgNum, d, err)
	}
	intf, err := cfg.Interface(0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to select interface #%d alternate setting %d of config %d of device %s: %v", 0, 0, cfgNum, d, err)
	}
	return intf, nil
}

// Close closes the device.
func (d *Device) Close() error {
	if d.handle == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.claimed != nil {
		return fmt.Errorf("can't release the device %s, it has an open config %s", d, d.claimed.Info.Config)
	}
	libusb.close(d.handle)
	d.handle = nil
	return nil
}

// GetStringDescriptor returns a device string descriptor with the given index
// number. The first supported language is always used and the returned
// descriptor string is converted to ASCII (non-ASCII characters are replaced
// with "?").
func (d *Device) GetStringDescriptor(descIndex int) (string, error) {
	if d.handle == nil {
		return "", fmt.Errorf("GetStringDescriptor(%d) called on %s after Close", descIndex, d)
	}
	return libusb.getStringDesc(d.handle, descIndex)
}

// SetAutoDetach enables/disables automatic kernel driver detachment.
// When autodetach is enabled gousb will automatically detach the kernel driver
// on the interface and reattach it when releasing the interface.
// Automatic kernel driver detachment is disabled on newly opened device handles by default.
func (d *Device) SetAutoDetach(autodetach bool) error {
	if d.handle == nil {
		return fmt.Errorf("SetAutoDetach(%v) called on %s after Close", autodetach, d)
	}
	var autodetachInt int
	switch autodetach {
	case true:
		autodetachInt = 1
	case false:
		autodetachInt = 0
	}
	return libusb.setAutoDetach(d.handle, autodetachInt)
}
