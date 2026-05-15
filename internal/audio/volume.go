package audio

import (
	"math"
	"time"
)

const volumeTau = 0.5 // seconds; time constant for RealVolume smoothing

// VolumeHelper smooths volume changes across frames and enforces a MaxVolume ceiling.
type VolumeHelper struct {
	TargetVolume float64
	RealVolume   float64
	MaxVolume    float64
}

// SetTargetVolume clamps vol to [0, MaxVolume] then stores it as the new target.
func (v *VolumeHelper) SetTargetVolume(vol float64) {
	if vol < 0 {
		vol = 0
	}
	if vol > v.MaxVolume {
		vol = v.MaxVolume
	}
	v.TargetVolume = vol
}

// Cycle advances RealVolume one step toward TargetVolume using exact exponential
// smoothing with time constant volumeTau:
//
//	RealVolume += (TargetVolume - RealVolume) * (1 - exp(-δ / τ))
//
// The exact form (not a linear approximation) is used so that large deltas — e.g.
// during a GC pause or reconnect — never overshoot.
func (v *VolumeHelper) Cycle(delta time.Duration) {
	alpha := 1 - math.Exp(-delta.Seconds()/volumeTau)
	v.RealVolume += (v.TargetVolume - v.RealVolume) * alpha
}
