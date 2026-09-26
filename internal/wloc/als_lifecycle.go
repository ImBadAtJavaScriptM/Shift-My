package wloc

import "errors"

// ALSLifecycleOutcome models the post-requester state transition observed in
// controlled unified-log traces. It intentionally begins after a response has
// already been parsed by ALS; it does not invoke private CoreLocation APIs.
type ALSLifecycleOutcome struct {
	Event      string
	Resolution string
	MatchedAPs int
	Estimate   *WifiPositionEstimate
}

// EvaluateALSLifecycle approximates the WifiPosition decision that follows a
// completed ALS requester. The traces show Network::AlsFinished occurring even
// when the response has no usable overlap with the live scan; the overlap
// decision happens afterward.
//
// A usable overlap produces a lab WifiPosition estimate and "fix". Zero usable
// overlap produces "all-unknown", corresponding to the observed
// Network::AlsAllUnknown path.
func EvaluateALSLifecycle(scan []ScanObservation, response []DeviceLocation, maxAPs int) (ALSLifecycleOutcome, error) {
	estimate, err := EstimateWifiPosition(scan, response, maxAPs)
	if err != nil {
		if errors.Is(err, ErrNoUsableBSSIDOverlap) {
			return ALSLifecycleOutcome{
				Event:      "Network::AlsFinished",
				Resolution: "Network::AlsAllUnknown",
				MatchedAPs: 0,
			}, nil
		}
		return ALSLifecycleOutcome{}, err
	}

	return ALSLifecycleOutcome{
		Event:      "Network::AlsFinished",
		Resolution: "fix",
		MatchedAPs: estimate.MatchedAPs,
		Estimate:   &estimate,
	}, nil
}
