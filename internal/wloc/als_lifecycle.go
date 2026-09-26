package wloc

import (
	"errors"
	"strings"
)

const ObservedALSWorkingSetHint = 18

// ALSLifecycleOutcome models the post-requester state transition observed in
// controlled unified-log traces. It intentionally begins after a response has
// already been parsed by ALS; it does not invoke private CoreLocation APIs.
type ALSLifecycleOutcome struct {
	Event                   string
	Resolution              string
	MatchedAPs              int
	TotalScan               int
	ALSLocated              int
	ReturnedWithoutLocation int
	NotReturned             int
	ALSLocatedPercent       int
	MissingPercent          int
	HasUsableOverlap        bool
	ObservedWorkingSetHint  int
	CandidateWorkingSet     int
	RejectedByWorkingSetCap int
	WorkingSetPercent       int
	Estimate                *WifiPositionEstimate
}

// EvaluateALSLifecycle approximates the WifiPosition decision that follows a
// completed ALS requester. The traces show Network::AlsFinished occurring even
// when the response has no usable overlap with the live scan; the overlap
// decision happens afterward.
//
// A usable overlap produces a lab WifiPosition estimate and "fix". Zero usable
// overlap produces "all-unknown", corresponding to the observed
// Network::AlsAllUnknown path.
//
// The classification fields intentionally distinguish only what the controlled
// parser can prove. ReturnedWithoutLocation means the response contained the
// BSSID but no usable Location. NotReturned means no record existed at all.
// The real private logs distinguish "unknown" and "notindb" more finely.
func EvaluateALSLifecycle(scan []ScanObservation, response []DeviceLocation, maxAPs int) (ALSLifecycleOutcome, error) {
	classification, err := classifyALSOverlap(scan, response)
	if err != nil {
		return ALSLifecycleOutcome{}, err
	}

	outcome := ALSLifecycleOutcome{
		Event:                   "Network::AlsFinished",
		Resolution:              "Network::AlsAllUnknown",
		MatchedAPs:              classification.ALSLocated,
		TotalScan:               classification.TotalScan,
		ALSLocated:              classification.ALSLocated,
		ReturnedWithoutLocation: classification.ReturnedWithoutLocation,
		NotReturned:             classification.NotReturned,
		ALSLocatedPercent:       classification.ALSLocatedPercent,
		MissingPercent:          classification.MissingPercent,
		HasUsableOverlap:        classification.ALSLocated > 0,
		ObservedWorkingSetHint:  ObservedALSWorkingSetHint,
		CandidateWorkingSet:     classification.ALSLocated,
	}
	if outcome.CandidateWorkingSet > ObservedALSWorkingSetHint {
		outcome.RejectedByWorkingSetCap = outcome.CandidateWorkingSet - ObservedALSWorkingSetHint
		outcome.CandidateWorkingSet = ObservedALSWorkingSetHint
	}
	outcome.WorkingSetPercent = roundedPercent(outcome.CandidateWorkingSet, outcome.TotalScan)
	if !outcome.HasUsableOverlap {
		return outcome, nil
	}

	estimate, err := EstimateWifiPosition(scan, response, maxAPs)
	if err != nil {
		if errors.Is(err, ErrNoUsableBSSIDOverlap) {
			// Defensive consistency with the classification path.
			outcome.HasUsableOverlap = false
			outcome.CandidateWorkingSet = 0
			return outcome, nil
		}
		return ALSLifecycleOutcome{}, err
	}

	outcome.Resolution = "fix"
	outcome.Estimate = &estimate
	return outcome, nil
}

type alsOverlapClassification struct {
	TotalScan               int
	ALSLocated              int
	ReturnedWithoutLocation int
	NotReturned             int
	ALSLocatedPercent       int
	MissingPercent          int
}

func classifyALSOverlap(scan []ScanObservation, response []DeviceLocation) (alsOverlapClassification, error) {
	uniqueScan := make(map[string]ScanObservation, len(scan))
	for _, observation := range scan {
		if !looksLikeBSSID(observation.BSSID) {
			continue
		}
		key := strings.ToLower(observation.BSSID)
		current, ok := uniqueScan[key]
		if !ok || observation.RSSI > current.RSSI {
			uniqueScan[key] = observation
		}
	}
	if len(uniqueScan) == 0 {
		return alsOverlapClassification{}, errors.New("scan contains no valid BSSIDs")
	}

	returned := make(map[string]DeviceLocation, len(response))
	for _, device := range response {
		if !looksLikeBSSID(device.BSSID) {
			continue
		}
		key := strings.ToLower(device.BSSID)
		current, ok := returned[key]
		if !ok || betterLocation(device, current) {
			returned[key] = device
		}
	}

	classification := alsOverlapClassification{TotalScan: len(uniqueScan)}
	for key := range uniqueScan {
		device, ok := returned[key]
		if !ok {
			classification.NotReturned++
			continue
		}
		if usableWifiLocation(device) {
			classification.ALSLocated++
			continue
		}
		classification.ReturnedWithoutLocation++
	}

	classification.ALSLocatedPercent = roundedPercent(classification.ALSLocated, classification.TotalScan)
	classification.MissingPercent = 100 - classification.ALSLocatedPercent
	return classification, nil
}

func roundedPercent(numerator, denominator int) int {
	if denominator <= 0 {
		return 0
	}
	return (numerator*100 + denominator/2) / denominator
}
