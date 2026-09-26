package wloc

import (
	"math"
	"testing"
)

func TestStepVerticalStateObservedMeasurementChange(t *testing.T) {
	previous := VerticalState{
		Altitude: 225.18428858767692,
		Variance: 5.5222543627444916,
	}
	got, err := StepVerticalState(
		previous,
		0.26296794414520264,
		225.18142258564598,
		4.027505414940953,
	)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.PredictedAltitude-225.18428858767692) > 1e-13 {
		t.Fatalf("predicted altitude=%0.15f", got.PredictedAltitude)
	}
	if math.Abs(got.PredictedVariance-5.523637405537451) > 1e-14 {
		t.Fatalf("predicted variance=%0.17f", got.PredictedVariance)
	}
	if math.Abs(got.KalmanGain-0.105409409) > 1e-9 {
		t.Fatalf("gain=%0.12f", got.KalmanGain)
	}
	if math.Abs(got.Posterior.Altitude-225.18398648409607) > 1e-12 {
		t.Fatalf("posterior altitude=%0.15f", got.Posterior.Altitude)
	}
	if math.Abs(got.Posterior.Variance-4.941394049979019) > 1e-14 {
		t.Fatalf("posterior variance=%0.17f", got.Posterior.Variance)
	}
}

func TestStepVerticalStateObservedLongGap(t *testing.T) {
	previous := VerticalState{
		Altitude: 225.09374206080156,
		Variance: 1.8905015060600145,
	}
	got, err := StepVerticalState(
		previous,
		15.186900973320007,
		225.00953746494892,
		2.6997176151520064,
	)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.PredictedVariance-6.503340729528579) > 1e-14 {
		t.Fatalf("predicted variance=%0.17f", got.PredictedVariance)
	}
	if math.Abs(got.Posterior.Altitude-225.0738773493281) > 1e-12 {
		t.Fatalf("posterior altitude=%0.15f", got.Posterior.Altitude)
	}
	if math.Abs(got.Posterior.Variance-4.969137211327856) > 1e-14 {
		t.Fatalf("posterior variance=%0.17f", got.Posterior.Variance)
	}
}

func TestInitializeVerticalState(t *testing.T) {
	got, err := InitializeVerticalState(100.5, 6.247955550297623)
	if err != nil {
		t.Fatal(err)
	}
	if got.Altitude != 100.5 || math.Abs(got.Variance-39.03694855849487) > 1e-14 {
		t.Fatalf("state=%+v", got)
	}
}

func TestStepVerticalStateRejectsInvalidInput(t *testing.T) {
	if _, err := StepVerticalState(VerticalState{Altitude: 1, Variance: 0}, 1, 2, 3); err == nil {
		t.Fatal("expected variance error")
	}
	if _, err := StepVerticalState(VerticalState{Altitude: math.NaN(), Variance: 1}, 1, 2, 3); err == nil {
		t.Fatal("expected altitude error")
	}
}
