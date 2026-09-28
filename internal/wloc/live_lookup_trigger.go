package wloc

import "strings"

const (
	ObservedLookupReasonUnknownRatio = "unknownratio"
	ObservedLookupReasonWSBLive      = "WSB_Live"
)

// LiveWifiLookupDecision describes one trace-supported reason for creating an
// ALS/WLOC query. It is a lab decision object only; evaluating it performs no
// networking and does not invoke private platform APIs.
type LiveWifiLookupDecision struct {
	ShouldQuery        bool
	Reason             string
	Pass               string
	TotalScan          int
	NotReturned        int
	NotReturnedPercent int
	RequestBSSIDs      []string
	OneShotSuppressed  bool
	Method             string
}

// LiveWifiLookupTriggerState tracks only the one-shot Stage2 transition proven
// by the two captured unknownratio flows. It intentionally does not invent a
// time-based retry or cooldown.
type LiveWifiLookupTriggerState struct {
	stage2UnknownRatioIssued bool
}

func (s *LiveWifiLookupTriggerState) ResetPositioningCycle() {
	s.stage2UnknownRatioIssued = false
}

// EvaluateStage2InitialFinal models the positive Stage2 transition observed in
// both genuine captures:
//
//	nextstage -> Stage2 -> finalscan|initialscan|inprogress
//	-> 100% not-in-db current scan -> unknownratio WLOC query
//
// Both captures show this transition exactly once. Later cached/provider passes
// can remain 100% unresolved without issuing another unknownratio query, so
// this path is modeled as one-shot per positioning cycle rather than as a
// generic unresolved-percentage trigger.
//
// The all-not-in-db requirement is the observed positive case, not a claim that
// Apple's private threshold is universally exactly 100%.
func (s *LiveWifiLookupTriggerState) EvaluateStage2InitialFinal(
	scan []ScanObservation,
	cached []DeviceLocation,
) (LiveWifiLookupDecision, error) {
	classification, err := classifyALSOverlap(scan, cached)
	if err != nil {
		return LiveWifiLookupDecision{}, err
	}

	decision := LiveWifiLookupDecision{
		Reason:             ObservedLookupReasonUnknownRatio,
		Pass:               "Stage2-initial-final",
		TotalScan:          classification.TotalScan,
		NotReturned:        classification.NotReturned,
		NotReturnedPercent: classification.NotReturnedPercent,
		RequestBSSIDs:      uniqueScanBSSIDs(scan),
		Method:             "observed-stage2-initial-final-unknownratio",
	}
	if s.stage2UnknownRatioIssued {
		decision.OneShotSuppressed = true
		return decision, nil
	}
	if classification.NotReturned != classification.TotalScan {
		return decision, nil
	}

	s.stage2UnknownRatioIssued = true
	decision.ShouldQuery = true
	return decision, nil
}

// EvaluateWSBLive models the separate Wifi::Wsb live-pass path observed later
// in one capture. The observed positive case was a three-AP scan group with all
// three APs not in the current ALS/tile lookup state, followed by a WSB_Live
// WLOC request.
//
// This path is intentionally stateless because the available traces do not
// establish a universal WSB retry/cooldown rule.
func EvaluateWSBLive(
	scan []ScanObservation,
	cached []DeviceLocation,
) (LiveWifiLookupDecision, error) {
	classification, err := classifyALSOverlap(scan, cached)
	if err != nil {
		return LiveWifiLookupDecision{}, err
	}
	decision := LiveWifiLookupDecision{
		Reason:             ObservedLookupReasonWSBLive,
		Pass:               "wsb-live",
		TotalScan:          classification.TotalScan,
		NotReturned:        classification.NotReturned,
		NotReturnedPercent: classification.NotReturnedPercent,
		RequestBSSIDs:      uniqueScanBSSIDs(scan),
		Method:             "observed-wsb-live-notindb",
	}
	if classification.NotReturned == classification.TotalScan {
		decision.ShouldQuery = true
	}
	return decision, nil
}

func uniqueScanBSSIDs(scan []ScanObservation) []string {
	strongest := make(map[string]ScanObservation, len(scan))
	order := make([]string, 0, len(scan))
	for _, observation := range scan {
		if !looksLikeBSSID(observation.BSSID) {
			continue
		}
		key := strings.ToLower(observation.BSSID)
		current, ok := strongest[key]
		if !ok {
			order = append(order, key)
			strongest[key] = observation
			continue
		}
		if observation.RSSI > current.RSSI {
			strongest[key] = observation
		}
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		observation, ok := strongest[key]
		if !ok {
			return nil
		}
		out = append(out, observation.BSSID)
	}
	return out
}
