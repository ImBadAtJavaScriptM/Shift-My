package wloc

import (
	"errors"
	"math"
)

const ObservedVerticalProcessVarianceCoefficient = 0.02

// VerticalConfidenceStep models one complete trace-supported confidence step:
// prediction from the previous posterior, followed by the Wi-Fi vertical
// measurement update.
type VerticalConfidenceStep struct {
	PreviousPosteriorVariance float64
	DeltaTime                 float64
	ProcessVariance           float64
	PredictedPriorVariance    float64
	Update                    VerticalConfidenceUpdate
	Method                    string
}

// InitializeVerticalConfidence converts the trace-exposed handoff sigma into
// the variance used by the subsequent scalar confidence recurrence.
func InitializeVerticalConfidence(handoffSigma float64) (float64, error) {
	if handoffSigma <= 0 || math.IsNaN(handoffSigma) || math.IsInf(handoffSigma, 0) {
		return 0, errors.New("handoff sigma must be finite and positive")
	}
	return handoffSigma * handoffSigma, nil
}

// PredictVerticalConfidence applies the observed scalar prediction rule:
//
//	Pprior = Pprevious + 0.02 * dt^2
//
// Across all 38 sequential decodable transitions in the two genuine captures,
// including transitions where the private epoch/base timestamp changes, this
// relation matched to floating-point precision.
func PredictVerticalConfidence(previousPosteriorVariance, deltaTime float64) (float64, error) {
	if previousPosteriorVariance <= 0 ||
		math.IsNaN(previousPosteriorVariance) ||
		math.IsInf(previousPosteriorVariance, 0) {
		return 0, errors.New("previous posterior variance must be finite and positive")
	}
	if deltaTime < 0 || math.IsNaN(deltaTime) || math.IsInf(deltaTime, 0) {
		return 0, errors.New("delta time must be finite and non-negative")
	}
	return previousPosteriorVariance +
		ObservedVerticalProcessVarianceCoefficient*deltaTime*deltaTime, nil
}

// StepVerticalConfidence combines the observed prediction rule with the
// measurement update implemented by FuseVerticalConfidence.
func StepVerticalConfidence(previousPosteriorVariance, deltaTime, rawMeasurementSigma float64) (VerticalConfidenceStep, error) {
	prior, err := PredictVerticalConfidence(previousPosteriorVariance, deltaTime)
	if err != nil {
		return VerticalConfidenceStep{}, err
	}
	update, err := FuseVerticalConfidence(prior, rawMeasurementSigma)
	if err != nil {
		return VerticalConfidenceStep{}, err
	}

	return VerticalConfidenceStep{
		PreviousPosteriorVariance: previousPosteriorVariance,
		DeltaTime:                 deltaTime,
		ProcessVariance:           prior - previousPosteriorVariance,
		PredictedPriorVariance:    prior,
		Update:                    update,
		Method:                    "observed-dt2-prediction-plus-precision-fusion",
	}, nil
}
