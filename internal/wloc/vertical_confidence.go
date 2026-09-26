package wloc

import (
	"errors"
	"math"
)

const ObservedVerticalMeasurementInflation = 1.7

// VerticalConfidenceUpdate models the trace-supported scalar confidence update
// that follows the instantaneous Wi-Fi vertical solve. It deliberately accepts
// an externally supplied prior variance: initialization, prediction/process
// noise, and reset behavior remain stateful/private and are not inferred here.
type VerticalConfidenceUpdate struct {
	PriorVariance       float64
	RawMeasurementSigma float64
	MeasurementSigma    float64
	MeasurementVariance float64
	PosteriorVariance   float64
	PosteriorSigma      float64
	Method              string
}

// FuseVerticalConfidence applies the inverse-variance update observed in the
// controlled traces:
//
//	measurementSigma = 1.7 * rawMeasurementSigma
//	R = measurementSigma^2
//	Pposterior = 1 / (1/Pprior + 1/R)
//
// Across 26 visible updates, the 1.7 inflation factor was exact to the decoded
// double precision. This helper does not model how CoreLocation chooses or
// predicts Pprior between provider passes.
func FuseVerticalConfidence(priorVariance, rawMeasurementSigma float64) (VerticalConfidenceUpdate, error) {
	if priorVariance <= 0 || math.IsNaN(priorVariance) || math.IsInf(priorVariance, 0) {
		return VerticalConfidenceUpdate{}, errors.New("prior variance must be finite and positive")
	}
	if rawMeasurementSigma <= 0 || math.IsNaN(rawMeasurementSigma) || math.IsInf(rawMeasurementSigma, 0) {
		return VerticalConfidenceUpdate{}, errors.New("raw measurement sigma must be finite and positive")
	}

	measurementSigma := ObservedVerticalMeasurementInflation * rawMeasurementSigma
	measurementVariance := measurementSigma * measurementSigma
	posteriorVariance := 1 / (1/priorVariance + 1/measurementVariance)

	return VerticalConfidenceUpdate{
		PriorVariance:       priorVariance,
		RawMeasurementSigma: rawMeasurementSigma,
		MeasurementSigma:    measurementSigma,
		MeasurementVariance: measurementVariance,
		PosteriorVariance:   posteriorVariance,
		PosteriorSigma:      math.Sqrt(posteriorVariance),
		Method:              "observed-1.7x-inflated-precision-fusion",
	}, nil
}
