package control

import (
	"math"
	"testing"
)

func TestFormulaCAsymmetricRecoveryIsLogarithmic(t *testing.T) {
	k := &ACCMathKernel{}

	at0 := k.CalculateRecovery(0)
	if at0 != CVPCritical {
		t.Fatalf("Δt=0: want CVPCritical=%v, got %v", CVPCritical, at0)
	}

	early := k.CalculateRecovery(10)
	mid := k.CalculateRecovery(KProbationEpochs)
	late := k.CalculateRecovery(KProbationEpochs * 4)

	if !(early > at0 && mid > early && late > mid) {
		t.Fatalf("recovery must be strictly increasing: 0=%v 10=%v mid=%v late=%v", at0, early, mid, late)
	}

	// Marginal gain shrinks: (mid-early)/(128-10) ≫ (late-mid)/(512-128) is not required,
	// but late-mid must be smaller than mid-early for equal-ish spans — logarithmic shape.
	gainEarly := mid - early
	gainLate := late - mid
	if gainLate >= gainEarly {
		t.Fatalf("expected diminishing returns: earlyGain=%v lateGain=%v", gainEarly, gainLate)
	}

	expectedMid := CVPCritical + float64(Kappa)*math.Log1p(float64(KProbationEpochs))
	if math.Abs(mid-expectedMid) > 1e-9 {
		t.Fatalf("Formula C mismatch: got %v want %v", mid, expectedMid)
	}
}

func TestFormulaCNeverJumpsToPermissiveFloorAlone(t *testing.T) {
	k := &ACCMathKernel{}
	// With κ=0.02, Formula C alone stays well below 0.8 at KProbationEpochs —
	// ACC-evolved CVPScore must still clear CVPPermissiveFloor (asymmetric coupling).
	atProbation := k.CalculateRecovery(KProbationEpochs)
	if atProbation >= float64(CVPPermissiveFloor) {
		t.Fatalf("Formula C alone reached permissive floor too fast: %v", atProbation)
	}
}
