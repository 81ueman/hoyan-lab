package device

import "github.com/81ueman/hoyan-lab/internal/core/topology"

type DeviceProfile = topology.DeviceProfile
type InterfaceProfile = topology.InterfaceProfile
type ACLProfile = topology.ACLProfile
type FIBProfile = topology.FIBProfile
type LiveProfile = topology.LiveProfile
type ConfigProfile = topology.ConfigProfile
type LiveCollectorID = topology.LiveCollectorID

const (
	LiveCollectorFRR     = topology.LiveCollectorFRR
	LiveCollectorCEOS    = topology.LiveCollectorCEOS
	LiveCollectorSRLinux = topology.LiveCollectorSRLinux
)

var (
	ProfileFor            = topology.ProfileFor
	RegisterDeviceProfile = topology.RegisterDeviceProfile
	RegisteredDeviceKinds = topology.RegisteredDeviceKinds
)
