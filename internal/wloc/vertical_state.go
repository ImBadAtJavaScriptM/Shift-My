package wloc

import (
	"errors"
	"math"
)

// VerticalState is the trace-supported scalar altitude state used by the lab
// model. It intentionally models only the visible altitude mean and variance.
type VerticalState struct {
	Altitude float64
	Variance float64
}

// VerticalStateStep exposes one complete predict/update cycle.
type VerticalStateStep struct {
	Previous            VerticalState
	DeltaTime           float64
	PredictedAltitude   float64
	PredictedVariance   float64
	MeasurementAltitude float64
	RawMeasurementSigma float64
	MeasurementSigma    float64
	MeasurementVariance float64
	KalmanGain          float64
	Posterior           VerticalState
	Method              string
}

// InitializeVerticalState seeds the scalar state from the handoff altitude and
// sigma visible in the trace.
func InitializeVerticalState(altitude, handoffSigma float64) (VerticalState, error) {
	if math.IsNaN(altitude) || math.IsInf(altitude, 0) {
		return VerticalState{}, errors.New("altitude must be finite")
	}
	variance, err := InitializeVerticalConfidence(handoffSigma)
	if err != nil {
		return VerticalState{}, err
	}
	return VerticalState{Altitude: altitude, Variance: variance}, nil
}

// StepVerticalState applies the complete trace-supported scalar recurrence:
//
//	predictedAltitude = previousAltitude
//	predictedVariance = previousVariance + 0.02 * dt^2
//	R = (1.7 * rawMeasurementSigma)^2
//	K = predictedVariance / (predictedVariance + R)
//	posteriorAltitude = predictedAltitude + K*(measurementAltitude-predictedAltitude)
//	posteriorVariance = (1-K)*predictedVariance
//
// The separate internal constant 0.0005 visible beside the 0.02 coefficient
// is intentionally not used because it is not required to reproduce the
// observed scalar altitude/variance state.
func StepVerticalState(previous VerticalState, deltaTime, measurementAltitude, rawMeasurementSigma float64) (VerticalStateStep, error) {
	if math.IsNaN(previous.Altitude) || math.IsInf(previous.Altitude, 0) {
		return VerticalStateStep{}, errors.New("previous altitude must be finite")
	}
	if previous.Variance <= 0 || math.IsNaN(previous.Variance) || math.IsInf(previous.Variance, 0) {
		return VerticalStateStep{}, errors.New("previous variance must be finite and positive")
	}
	if math.IsNaN(measurementAltitude) || math.IsInf(measurementAltitude, 0) {
		return VerticalStateStep{}, errors.New("measurement altitude must be finite")
	}

	predictedVariance, err := PredictVerticalConfidence(previous.Variance, deltaTime)
	if err != nil {
		return VerticalStateStep{}, err
	}
	update, err := FuseVerticalConfidence(predictedVariance, rawMeasurementSigma)
	if err != nil {
		return VerticalStateStep{}, err
	}

	kalmanGain := predictedVariance / (predictedVariance + update.MeasurementVariance)
	posteriorAltitude := previous.Altitude +
		kalmanGain*(measurementAltitude-previous.Altitude)

	return VerticalStateStep{
		Previous:            previous,
		DeltaTime:           deltaTime,
		PredictedAltitude:   previous.Altitude,
		PredictedVariance:   predictedVariance,
		MeasurementAltitude: measurementAltitude,
		RawMeasurementSigma: rawMeasurementSigma,
		MeasurementSigma:    update.MeasurementSigma,
		MeasurementVariance: update.MeasurementVariance,
		KalmanGain:          kalmanGain,
		Posterior: VerticalState{
			Altitude: posteriorAltitude,
			Variance: update.PosteriorVariance,
		},
		Method: "observed-scalar-altitude-kalman",
	}, nil
}
