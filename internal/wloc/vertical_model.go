package wloc

import (
	"errors"
	"sort"
	"strings"
)

const (
	DefaultWifiVerticalMaxAPs       = 10
	ObservedVerticalAccuracyCeiling = 4.0
)

// WifiVerticalEstimate models only the trace-supported instantaneous altitude
// solve. Reported CoreLocation vertical accuracy is deliberately excluded
// because the captures show it depends on state across repeated provider passes.
type WifiVerticalEstimate struct {
	Altitude        float64
	MatchedAPs      int
	QualifiedAPs    int
	UsedAPs         int
	MaxAPs          int
	StrongestRSSI   int
	WeakestUsedRSSI int
	Method          string
}

type matchedVerticalObservation struct {
	BSSID            string
	RSSI             int
	Altitude         float64
	VerticalAccuracy float64
}

// EstimateWifiVerticalPosition approximates the private vertical measurement
// stage observed in the controlled traces:
//
//  1. deduplicate scan BSSIDs, retaining the strongest RSSI;
//  2. match them to response entries with valid altitude metadata;
//  3. retain high-quality entries whose vertical accuracy is <= 4 m;
//  4. cap that vertical working set at the strongest 10 observations;
//  5. compute an RSSI-weighted altitude with weight max(1, 100+RSSI).
//
// Across 40 trace-exposed internal vertical solves, the corresponding rule had
// about 7.9 cm mean absolute error and under 19 cm worst-case error. Raw WLOC
// altitude fields are integer meters, so wire-level fixtures are inherently
// coarser. This is an empirical lab approximation, not a claim about Apple's
// private implementation.
func EstimateWifiVerticalPosition(scan []ScanObservation, response []DeviceLocation, maxAPs int) (WifiVerticalEstimate, error) {
	if maxAPs <= 0 {
		maxAPs = DefaultWifiVerticalMaxAPs
	}

	strongest := make(map[string]ScanObservation, len(scan))
	for _, observation := range scan {
		if !looksLikeBSSID(observation.BSSID) {
			continue
		}
		key := strings.ToLower(observation.BSSID)
		current, ok := strongest[key]
		if !ok || observation.RSSI > current.RSSI {
			strongest[key] = observation
		}
	}
	if len(strongest) == 0 {
		return WifiVerticalEstimate{}, errors.New("scan contains no valid BSSIDs")
	}

	locations := make(map[string]DeviceLocation, len(response))
	for _, device := range response {
		if !looksLikeBSSID(device.BSSID) {
			continue
		}
		key := strings.ToLower(device.BSSID)
		current, ok := locations[key]
		if !ok || betterVerticalLocation(device, current) {
			locations[key] = device
		}
	}

	matches := make([]matchedVerticalObservation, 0, len(strongest))
	qualified := make([]matchedVerticalObservation, 0, len(strongest))
	for key, observation := range strongest {
		device, ok := locations[key]
		if !ok || !usableVerticalLocation(device) {
			continue
		}
		match := matchedVerticalObservation{
			BSSID:            observation.BSSID,
			RSSI:             observation.RSSI,
			Altitude:         float64(device.Altitude),
			VerticalAccuracy: float64(device.VerticalAccuracy),
		}
		matches = append(matches, match)
		if match.VerticalAccuracy <= ObservedVerticalAccuracyCeiling {
			qualified = append(qualified, match)
		}
	}
	if len(qualified) == 0 {
		return WifiVerticalEstimate{}, errors.New("scan and response have no high-quality vertical overlap")
	}

	sort.Slice(qualified, func(i, j int) bool {
		if qualified[i].RSSI != qualified[j].RSSI {
			return qualified[i].RSSI > qualified[j].RSSI
		}
		return strings.ToLower(qualified[i].BSSID) < strings.ToLower(qualified[j].BSSID)
	})

	used := qualified
	if len(used) > maxAPs {
		used = used[:maxAPs]
	}

	var altitude, totalWeight float64
	for _, match := range used {
		weight := float64(100 + match.RSSI)
		if weight < 1 {
			weight = 1
		}
		altitude += match.Altitude * weight
		totalWeight += weight
	}
	altitude /= totalWeight

	return WifiVerticalEstimate{
		Altitude:        altitude,
		MatchedAPs:      len(matches),
		QualifiedAPs:    len(qualified),
		UsedAPs:         len(used),
		MaxAPs:          maxAPs,
		StrongestRSSI:   used[0].RSSI,
		WeakestUsedRSSI: used[len(used)-1].RSSI,
		Method:          "empirical-vacc4-top10-rssi-weighted",
	}, nil
}

func usableVerticalLocation(device DeviceLocation) bool {
	// Captured sentinel records use altitude=-500 m and vertical accuracy=-1.
	// Public WLOC decoders expose these protobuf fields directly in meters; only
	// latitude/longitude use the 1e8 coordinate scaling.
	return device.Altitude > -100000 && device.VerticalAccuracy > 0
}

func betterVerticalLocation(candidate, current DeviceLocation) bool {
	if !usableVerticalLocation(candidate) {
		return false
	}
	if !usableVerticalLocation(current) {
		return true
	}
	return candidate.VerticalAccuracy < current.VerticalAccuracy
}
