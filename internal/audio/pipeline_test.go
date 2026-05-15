package audio

import (
	"math"
	"testing"
	"time"
)

// --- VolumeHelper.Cycle ---

func TestCycle_ConvergesOnTarget(t *testing.T) {
	v := VolumeHelper{TargetVolume: 0.8, RealVolume: 0.0, MaxVolume: 1.0}
	// Simulate 10 seconds of frames at 10ms each.
	for range 1000 {
		v.Cycle(10 * time.Millisecond)
	}
	if math.Abs(v.RealVolume-v.TargetVolume) > 0.001 {
		t.Errorf("did not converge: RealVolume=%.6f, want ≈%.6f", v.RealVolume, v.TargetVolume)
	}
}

func TestCycle_ExactFormula(t *testing.T) {
	v := VolumeHelper{TargetVolume: 1.0, RealVolume: 0.0, MaxVolume: 1.0}
	delta := 100 * time.Millisecond
	v.Cycle(delta)
	want := 1.0 - math.Exp(-delta.Seconds()/volumeTau)
	if math.Abs(v.RealVolume-want) > 1e-12 {
		t.Errorf("Cycle(%v): got %.12f, want %.12f", delta, v.RealVolume, want)
	}
}

func TestCycle_LargeDeltaDoesNotOvershoot(t *testing.T) {
	v := VolumeHelper{TargetVolume: 0.5, RealVolume: 1.0, MaxVolume: 1.0}
	// A 1-minute delta should not push RealVolume below TargetVolume.
	v.Cycle(time.Minute)
	if v.RealVolume < v.TargetVolume {
		t.Errorf("overshot: RealVolume=%.6f < TargetVolume=%.6f", v.RealVolume, v.TargetVolume)
	}
}

// --- VolumeHelper.SetTargetVolume ---

func TestSetTargetVolume_ClampAboveMax(t *testing.T) {
	v := VolumeHelper{MaxVolume: 0.9}
	v.SetTargetVolume(1.5)
	if v.TargetVolume != 0.9 {
		t.Errorf("expected TargetVolume=0.9, got %.2f", v.TargetVolume)
	}
}

func TestSetTargetVolume_ClampBelowZero(t *testing.T) {
	v := VolumeHelper{MaxVolume: 1.0}
	v.SetTargetVolume(-0.5)
	if v.TargetVolume != 0.0 {
		t.Errorf("expected TargetVolume=0.0, got %.2f", v.TargetVolume)
	}
}

func TestSetTargetVolume_WithinRange(t *testing.T) {
	v := VolumeHelper{MaxVolume: 1.0}
	v.SetTargetVolume(0.6)
	if v.TargetVolume != 0.6 {
		t.Errorf("expected TargetVolume=0.6, got %.2f", v.TargetVolume)
	}
}

// --- fade envelope ---

func TestFadeInMultiplier_Boundaries(t *testing.T) {
	// At x=0 the fade-in starts quiet (not silent, ~0.37).
	got := fadeInMultiplier(0)
	if got < 0.3 || got > 0.4 {
		t.Errorf("fadeInMultiplier(0)=%.4f, want ≈0.368", got)
	}
	// At x=fadeDuration it returns exactly 1.0.
	if v := fadeInMultiplier(fadeDuration); v != 1.0 {
		t.Errorf("fadeInMultiplier(%d)=%.4f, want 1.0", fadeDuration, v)
	}
	// Values are monotonically increasing.
	prev := fadeInMultiplier(0)
	for x := 1; x < fadeDuration; x++ {
		curr := fadeInMultiplier(x)
		if curr <= prev {
			t.Errorf("fadeInMultiplier not monotone at x=%d: %.4f <= %.4f", x, curr, prev)
		}
		prev = curr
	}
}

func TestFadeOutMultiplier_Boundaries(t *testing.T) {
	// At x=0 the fade-out starts at full volume.
	if v := fadeOutMultiplier(0); v != 1.0 {
		t.Errorf("fadeOutMultiplier(0)=%.4f, want 1.0", v)
	}
	// At x=fadeDuration it returns 0.0.
	if v := fadeOutMultiplier(fadeDuration); v != 0.0 {
		t.Errorf("fadeOutMultiplier(%d)=%.4f, want 0.0", fadeDuration, v)
	}
	// Values are monotonically decreasing.
	prev := fadeOutMultiplier(0)
	for x := 1; x < fadeDuration; x++ {
		curr := fadeOutMultiplier(x)
		if curr >= prev {
			t.Errorf("fadeOutMultiplier not monotone at x=%d: %.4f >= %.4f", x, curr, prev)
		}
		prev = curr
	}
}

func TestFadeMultiplier_KnownValues(t *testing.T) {
	// fadeOutMultiplier(0) == 1.0 (full volume at start of fade-out)
	if v := fadeOutMultiplier(0); v != 1.0 {
		t.Errorf("fadeOutMultiplier(0)=%.6f, want 1.0", v)
	}
	// At frame 30 (half of fadeDuration), both multipliers should equal exp(-0.5).
	wantOut30 := math.Exp(-30.0 / float64(fadeDuration))
	if v := fadeOutMultiplier(30); math.Abs(v-wantOut30) > 1e-12 {
		t.Errorf("fadeOutMultiplier(30)=%.12f, want %.12f", v, wantOut30)
	}
	wantIn30 := math.Exp(-float64(fadeDuration-30) / float64(fadeDuration))
	if v := fadeInMultiplier(30); math.Abs(v-wantIn30) > 1e-12 {
		t.Errorf("fadeInMultiplier(30)=%.12f, want %.12f", v, wantIn30)
	}
	// fadeInMultiplier(x) at x=0 equals fadeOutMultiplier at the matching exponent.
	wantIn0 := math.Exp(-1.0)
	if v := fadeInMultiplier(0); math.Abs(v-wantIn0) > 1e-12 {
		t.Errorf("fadeInMultiplier(0)=%.12f, want exp(-1)=%.12f", v, wantIn0)
	}
}

// --- applyScalar ---

func TestApplyScalar_HalfVolume(t *testing.T) {
	samples := []int16{1000, -1000, 2000}
	applyScalar(samples, 0.5)
	want := []int16{500, -500, 1000}
	for i, w := range want {
		if samples[i] != w {
			t.Errorf("samples[%d]=%d, want %d", i, samples[i], w)
		}
	}
}

func TestApplyScalar_Zero(t *testing.T) {
	samples := []int16{32767, -32768, 100}
	applyScalar(samples, 0.0)
	for i, s := range samples {
		if s != 0 {
			t.Errorf("samples[%d]=%d, want 0 after zero scalar", i, s)
		}
	}
}

func TestApplyScalar_ClampPositive(t *testing.T) {
	// Scalar > 1 should clamp, not overflow.
	samples := []int16{math.MaxInt16}
	applyScalar(samples, 2.0)
	if samples[0] != math.MaxInt16 {
		t.Errorf("expected clamped MaxInt16, got %d", samples[0])
	}
}

func TestApplyScalar_ClampNegative(t *testing.T) {
	samples := []int16{math.MinInt16}
	applyScalar(samples, 2.0)
	if samples[0] != math.MinInt16 {
		t.Errorf("expected clamped MinInt16, got %d", samples[0])
	}
}

func TestApplyScalar_Identity(t *testing.T) {
	samples := []int16{100, -200, 0, 32767, -32768}
	orig := make([]int16, len(samples))
	copy(orig, samples)
	applyScalar(samples, 1.0)
	for i, o := range orig {
		if samples[i] != o {
			t.Errorf("samples[%d]=%d, want %d with scalar=1.0", i, samples[i], o)
		}
	}
}
