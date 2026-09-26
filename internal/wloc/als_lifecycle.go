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
	Event                          string
	Resolution                     string
	MatchedAPs                     int
	TotalScan                      int
	ALSLocated                     int
	TileLocated                    int
	ReturnedWithoutLocation        int
	NotReturned                    int
	ALSLocatedPercent              int
	TileLocatedPercent             int
	ReturnedWithoutLocationPercent int
	NotReturnedPercent             int
	MissingPercent                 int
	HasUsableOverlap               bool
	ObservedWorkingSetHint         int
	CandidateWorkingSet            int
	RejectedByWorkingSetCap        int
	WorkingSetPercent              int
	Estimate                       *WifiPositionEstimate
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
		Event:                          "Network::AlsFinished",
		Resolution:                     "Network::AlsAllUnknown",
		MatchedAPs:                     classification.ALSLocated,
		TotalScan:                      classification.TotalScan,
		ALSLocated:                     classification.ALSLocated,
		TileLocated:                    classification.TileLocated,
		ReturnedWithoutLocation:        classification.ReturnedWithoutLocation,
		NotReturned:                    classification.NotReturned,
		ALSLocatedPercent:              classification.ALSLocatedPercent,
		TileLocatedPercent:             classification.TileLocatedPercent,
		ReturnedWithoutLocationPercent: classification.ReturnedWithoutLocationPercent,
		NotReturnedPercent:             classification.NotReturnedPercent,
		MissingPercent:                 classification.MissingPercent,
		HasUsableOverlap:               classification.ALSLocated > 0,
		ObservedWorkingSetHint:         ObservedALSWorkingSetHint,
		CandidateWorkingSet:            classification.ALSLocated,
	}
	if outcome.CandidateWorkingSet > ObservedALSWorkingSetHint {
		outcome.RejectedByWorkingSetCap = outcome.CandidateWorkingSet - ObservedALSWorkingSetHint
		outcome.CandidateWorkingSet = ObservedALSWorkingSetHint
	}
	outcome.WorkingSetPercent = floorPercent(outcome.CandidateWorkingSet, outcome.TotalScan)
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
	TotalScan                      int
	ALSLocated                     int
	TileLocated                    int
	ReturnedWithoutLocation        int
	NotReturned                    int
	ALSLocatedPercent              int
	TileLocatedPercent             int
	ReturnedWithoutLocationPercent int
	NotReturnedPercent             int
	MissingPercent                 int
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

	classification.ALSLocatedPercent = floorPercent(classification.ALSLocated, classification.TotalScan)
	classification.TileLocatedPercent = floorPercent(classification.TileLocated, classification.TotalScan)
	classification.ReturnedWithoutLocationPercent = floorPercent(classification.ReturnedWithoutLocation, classification.TotalScan)
	classification.NotReturnedPercent = floorPercent(classification.NotReturned, classification.TotalScan)
	classification.MissingPercent = floorPercent(
		classification.ReturnedWithoutLocation+classification.NotReturned,
		classification.TotalScan,
	)
	return classification, nil
}

// TraceSourcePercentages returns the four source buckets observed in the
// private WifiPosition tilesals tuple: ALS, tile, unknown, not-in-db.
//
// The controlled lab does not yet feed an independent tile-location source, so
// TileLocatedPercent is currently zero. Percentages are independently floored,
// matching the trace even when the four integers sum to 98 or 99.
func (o ALSLifecycleOutcome) TraceSourcePercentages() [4]int {
	return [4]int{
		o.ALSLocatedPercent,
		o.TileLocatedPercent,
		o.ReturnedWithoutLocationPercent,
		o.NotReturnedPercent,
	}
}

func floorPercent(numerator, denominator int) int {
	if denominator <= 0 {
		return 0
	}
	return numerator * 100 / denominator
}
