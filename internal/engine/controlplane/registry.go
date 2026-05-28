package controlplane

import (
	"sync"

	"github.com/81ueman/hoyan-lab/internal/domain/device"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

var behaviorRegistry = map[model.DeviceKind]device.DeviceBehavior{
	model.KindFRR:     device.NewFRRBehavior(),
	model.KindCEOS:    device.NewCEOSBehavior(),
	model.KindSRLinux: device.NewSRLinuxBehavior(),
}

var behaviorRegistryMu sync.RWMutex

func RegisterBehavior(kind model.DeviceKind, behavior device.DeviceBehavior) func() {
	behaviorRegistryMu.Lock()
	defer behaviorRegistryMu.Unlock()
	old, hadOld := behaviorRegistry[kind]
	behaviorRegistry[kind] = behavior
	return func() {
		behaviorRegistryMu.Lock()
		defer behaviorRegistryMu.Unlock()
		if hadOld {
			behaviorRegistry[kind] = old
			return
		}
		delete(behaviorRegistry, kind)
	}
}

func BehaviorFor(kind model.DeviceKind) device.DeviceBehavior {
	return behaviorFor(kind)
}

func behaviorFor(kind model.DeviceKind) device.DeviceBehavior {
	behaviorRegistryMu.RLock()
	b, ok := behaviorRegistry[kind]
	behaviorRegistryMu.RUnlock()
	if ok {
		return b
	}
	return device.NewGenericBehavior(kind)
}
