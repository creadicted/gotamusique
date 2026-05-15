package audio

import "math"

// fadeDuration is the number of frames over which fade-in and fade-out run.
// At 10ms per frame (gumble's AudioDefaultInterval) this is 600ms.
const fadeDuration = 60

// fadeInMultiplier returns the envelope scalar for frame x of a fade-in.
// At x=0 the multiplier is exp(-1) ≈ 0.37; at x=fadeDuration it returns 1.0.
func fadeInMultiplier(x int) float64 {
	if x >= fadeDuration {
		return 1.0
	}
	return math.Exp(-float64(fadeDuration-x) / float64(fadeDuration))
}

// fadeOutMultiplier returns the envelope scalar for frame x of a fade-out.
// At x=0 the multiplier is 1.0; at x=fadeDuration it returns exp(-1) ≈ 0.37.
func fadeOutMultiplier(x int) float64 {
	if x >= fadeDuration {
		return 0.0
	}
	return math.Exp(-float64(x) / float64(fadeDuration))
}

// applyScalar multiplies every sample in-place by scalar, clamping to int16 range.
func applyScalar(samples []int16, scalar float64) {
	for i, s := range samples {
		v := float64(s) * scalar
		if v > math.MaxInt16 {
			v = math.MaxInt16
		} else if v < math.MinInt16 {
			v = math.MinInt16
		}
		samples[i] = int16(v)
	}
}
