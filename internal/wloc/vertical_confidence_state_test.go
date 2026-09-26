package wloc

import (
	"math"
	"testing"
)

func TestInitializeVerticalConfidenceObservedHandoff(t *testing.T) {
	got, err := InitializeVerticalConfidence(6.247955550297623)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-39.03694855849487) > 1e-14 {
		t.Fatalf("variance=%0.17f", got)
	}
}

func TestStepVerticalConfidenceObservedSequence(t *testing.T) {
	initial, err := InitializeVerticalConfidence(6.247955550297623)
	if err != nil {
		t.Fatal(err)
	}

	first, err := StepVerticalConfidence(
		initial,
		0.2803150415420532,
		2.9836493952825407,
	)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(first.PredictedPriorVariance-39.03852008894516) > 1e-14 {
		t.Fatalf("first prior=%0.17f", first.PredictedPriorVariance)
	}
	if math.Abs(first.ProcessVariance-0.001571530450291902) > 1e-14 {
		t.Fatalf("first Q=%0.17f", first.ProcessVariance)
	}
	if math.Abs(first.Update.PosteriorVariance-15.507479310606902) > 1e-14 {
		t.Fatalf("first posterior=%0.17f", first.Update.PosteriorVariance)
	}

	second, err := StepVerticalConfidence(
		first.Update.PosteriorVariance,
		0.11354005336761475,
		2.9836493952825407,
	)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(second.PredictedPriorVariance-15.507737137481277) > 1e-14 {
		t.Fatalf("second prior=%0.17f", second.PredictedPriorVariance)
	}
	if math.Abs(second.Update.PosteriorVariance-9.675556517441224) > 1e-14 {
		t.Fatalf("second posterior=%0.17f", second.Update.PosteriorVariance)
	}
}

func TestPredictVerticalConfidenceLongGap(t *testing.T) {
	got, err := PredictVerticalConfidence(1.8905015060600145, 15.186900973320007)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-6.503340729528579) > 1e-14 {
		t.Fatalf("prior=%0.17f", got)
	}
}

func TestVerticalConfidenceStateRejectsInvalidInput(t *testing.T) {
	if _, err := InitializeVerticalConfidence(0); err == nil {
		t.Fatal("expected handoff error")
	}
	if _, err := PredictVerticalConfidence(1, -1); err == nil {
		t.Fatal("expected delta-time error")
	}
	if _, err := StepVerticalConfidence(0, 1, 2); err == nil {
		t.Fatal("expected previous-variance error")
	}
}
