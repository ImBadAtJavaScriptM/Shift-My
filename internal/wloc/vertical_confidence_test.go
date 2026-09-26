package wloc

import (
	"math"
	"testing"
)

func TestFuseVerticalConfidenceObservedSequenceA(t *testing.T) {
	// An observed provider state carried Pprior ~= 15.507479 m^2 while the
	// instantaneous AP vertical solve reported sigma ~= 2.983649 m.
	got, err := FuseVerticalConfidence(15.5074793, 2.9836493952825407)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.MeasurementSigma-5.072203971980319) > 1e-12 {
		t.Fatalf("measurement sigma=%0.15f", got.MeasurementSigma)
	}
	if math.Abs(got.MeasurementVariance-25.72725313) > 1e-7 {
		t.Fatalf("measurement variance=%0.12f", got.MeasurementVariance)
	}
	if math.Abs(got.PosteriorSigma-3.11054) > 0.001 {
		t.Fatalf("posterior sigma=%0.8f", got.PosteriorSigma)
	}
}

func TestFuseVerticalConfidenceObservedSequenceB(t *testing.T) {
	got, err := FuseVerticalConfidence(4.941394, 4.027505414940953)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.MeasurementVariance-46.87811162) > 1e-7 {
		t.Fatalf("measurement variance=%0.12f", got.MeasurementVariance)
	}
	if math.Abs(got.PosteriorSigma-2.11429) > 0.001 {
		t.Fatalf("posterior sigma=%0.8f", got.PosteriorSigma)
	}
}

func TestFuseVerticalConfidenceObservedInflationFactor(t *testing.T) {
	for _, sigma := range []float64{
		2.9836493952825407,
		4.027505414940953,
		2.6997176151520064,
	} {
		got, err := FuseVerticalConfidence(10, sigma)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got.MeasurementSigma/sigma-ObservedVerticalMeasurementInflation) > 1e-15 {
			t.Fatalf("sigma=%f factor=%0.17f", sigma, got.MeasurementSigma/sigma)
		}
	}
}

func TestFuseVerticalConfidenceRejectsInvalidInput(t *testing.T) {
	if _, err := FuseVerticalConfidence(0, 3); err == nil {
		t.Fatal("expected invalid prior variance error")
	}
	if _, err := FuseVerticalConfidence(4, 0); err == nil {
		t.Fatal("expected invalid measurement sigma error")
	}
}
