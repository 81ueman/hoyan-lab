package controlplane

import (
	"sync"

	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

var behaviorRegistry = map[topology.DeviceKind]DeviceBehavior{
	topology.KindFRR:     NewFRRBehavior(),
	topology.KindCEOS:    NewCEOSBehavior(),
	topology.KindSRLinux: NewSRLinuxBehavior(),
}

var behaviorRegistryMu sync.RWMutex

func RegisterBehavior(kind topology.DeviceKind, behavior DeviceBehavior) func() {
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

func BehaviorFor(kind topology.DeviceKind) DeviceBehavior {
	return behaviorFor(kind)
}

func behaviorFor(kind topology.DeviceKind) DeviceBehavior {
	behaviorRegistryMu.RLock()
	b, ok := behaviorRegistry[kind]
	behaviorRegistryMu.RUnlock()
	if ok {
		return b
	}
	return NewGenericBehavior(kind)
}
