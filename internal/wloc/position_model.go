package wloc

import (
	"errors"
	"math"
	"sort"
	"strings"
)

const DefaultWifiPositionMaxAPs = 16

var ErrNoUsableBSSIDOverlap = errors.New("scan and response have no usable BSSID overlap")

// ScanObservation models the part of a live Wi-Fi scan that the empirical
// lab estimator needs: BSSID identity plus received signal strength.
type ScanObservation struct {
	BSSID string
	RSSI  int
}

// WifiPositionEstimate is a controlled-lab approximation of the WifiPosition
// solve observed after Network::AlsFinished. It is not a claim about Apple's
// exact proprietary implementation.
type WifiPositionEstimate struct {
	Latitude        float64
	Longitude       float64
	MatchedAPs      int
	UsedAPs         int
	MaxAPs          int
	StrongestRSSI   int
	WeakestUsedRSSI int
	SpreadMeters    float64
	Method          string
}

type matchedWifiObservation struct {
	BSSID     string
	RSSI      int
	Latitude  float64
	Longitude float64
}

// EstimateWifiPosition approximates the post-ALS Wi-Fi solve seen in the
// controlled iPhone traces:
//  1. deduplicate the live scan, keeping the strongest observation per BSSID;
//  2. match scan BSSIDs against response records that contain usable locations;
//  3. retain at most the strongest maxAPs matches;
//  4. return the arithmetic centroid of the retained AP coordinates.
//
// Across two captured live WifiPosition solves, maxAPs=16 reproduced the
// observed fix to roughly 0.23 m and 1.08 m respectively. That makes this a
// useful lab model, not a bit-for-bit reconstruction of CoreLocation.
func EstimateWifiPosition(scan []ScanObservation, response []DeviceLocation, maxAPs int) (WifiPositionEstimate, error) {
	if maxAPs <= 0 {
		maxAPs = DefaultWifiPositionMaxAPs
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
		return WifiPositionEstimate{}, errors.New("scan contains no valid BSSIDs")
	}

	locations := make(map[string]DeviceLocation, len(response))
	for _, device := range response {
		if !looksLikeBSSID(device.BSSID) || !usableWifiLocation(device) {
			continue
		}
		key := strings.ToLower(device.BSSID)
		current, ok := locations[key]
		if !ok || betterLocation(device, current) {
			locations[key] = device
		}
	}

	matches := make([]matchedWifiObservation, 0, len(strongest))
	for key, observation := range strongest {
		device, ok := locations[key]
		if !ok {
			continue
		}
		matches = append(matches, matchedWifiObservation{
			BSSID:     observation.BSSID,
			RSSI:      observation.RSSI,
			Latitude:  float64(device.LatitudeE8) / 1e8,
			Longitude: float64(device.LongitudeE8) / 1e8,
		})
	}
	if len(matches) == 0 {
		return WifiPositionEstimate{}, ErrNoUsableBSSIDOverlap
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].RSSI != matches[j].RSSI {
			return matches[i].RSSI > matches[j].RSSI
		}
		return strings.ToLower(matches[i].BSSID) < strings.ToLower(matches[j].BSSID)
	})

	used := matches
	if len(used) > maxAPs {
		used = used[:maxAPs]
	}

	var latitude, longitude float64
	for _, match := range used {
		latitude += match.Latitude
		longitude += match.Longitude
	}
	latitude /= float64(len(used))
	longitude /= float64(len(used))

	spread := 0.0
	for _, match := range used {
		distance := approximateDistanceMeters(latitude, longitude, match.Latitude, match.Longitude)
		if distance > spread {
			spread = distance
		}
	}

	return WifiPositionEstimate{
		Latitude:        latitude,
		Longitude:       longitude,
		MatchedAPs:      len(matches),
		UsedAPs:         len(used),
		MaxAPs:          maxAPs,
		StrongestRSSI:   used[0].RSSI,
		WeakestUsedRSSI: used[len(used)-1].RSSI,
		SpreadMeters:    spread,
		Method:          "empirical-strongest-centroid",
	}, nil
}

func usableWifiLocation(device DeviceLocation) bool {
	latitude := float64(device.LatitudeE8) / 1e8
	longitude := float64(device.LongitudeE8) / 1e8
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return false
	}
	// A WifiDevice without a Location submessage parses to the all-zero
	// DeviceLocation fields. Treat that shape as missing rather than (0,0).
	if device.LatitudeE8 == 0 && device.LongitudeE8 == 0 && device.HorizontalAccuracy == 0 {
		return false
	}
	return true
}

func betterLocation(candidate, current DeviceLocation) bool {
	if candidate.HorizontalAccuracy <= 0 {
		return false
	}
	if current.HorizontalAccuracy <= 0 {
		return true
	}
	return candidate.HorizontalAccuracy < current.HorizontalAccuracy
}

func approximateDistanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const metersPerDegree = 111320.0
	meanLatitude := (lat1 + lat2) * math.Pi / 360
	dy := (lat2 - lat1) * metersPerDegree
	dx := (lon2 - lon1) * metersPerDegree * math.Cos(meanLatitude)
	return math.Hypot(dx, dy)
}
