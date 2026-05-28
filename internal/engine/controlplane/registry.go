package controlplane

import (
	"sync"

	deviceadapter "github.com/81ueman/hoyan-lab/internal/adapter/device"
	"github.com/81ueman/hoyan-lab/internal/domain/device"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

var behaviorRegistry = map[model.DeviceKind]device.DeviceBehavior{
	model.KindFRR:     deviceadapter.NewFRRBehavior(),
	model.KindCEOS:    deviceadapter.NewCEOSBehavior(),
	model.KindSRLinux: deviceadapter.NewSRLinuxBehavior(),
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
