package wloc

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ObservedALSResponseFamilyLive       = 0
	ObservedALSResponseFamilyBackground = 1
)

const (
	ALSQueryOriginLiveWifi               = "live-wifi"
	ALSQueryOriginCoordinateNeighborhood = "coordinate-neighborhood"
)

// ALSQueryOrigin models the high-level request identity visible in GeneralCLX
// before an ALS transport task is created. OriginID is distinct from both the
// later ALS issued serial and the CFNetwork task identifier.
//
// The genuine traces show two request families:
//   - live Wi-Fi requests with no coordinate center and an explicit reason
//     such as unknownratio or WSB_Live;
//   - coordinate-neighborhood requests used by background place/geofence work.
//
// This is a trace-correlation model only; it does not create network requests.
type ALSQueryOrigin struct {
	OriginID            int
	Kind                string
	Reason              string
	HasCoordinateCenter bool
}

// ALSResponseFamilySummary models the four-field GeneralCLX summary immediately
// preceding didReceiveResponse. FamilyFlag is an observed discriminator; 0 and
// 1 are descriptive trace values, not claimed Apple API constants.
type ALSResponseFamilySummary struct {
	OriginID    int
	RecordCount int
	FamilyFlag  int
}

// ALSConsumerRoute is the controlled-lab interpretation of a correlated origin
// and response summary.
type ALSConsumerRoute struct {
	Origin   ALSQueryOrigin
	Summary  ALSResponseFamilySummary
	Consumer string
	Method   string
}

// CorrelateALSConsumerRoute validates the trace-visible origin/family handoff.
//
// Across 30 mapped response callbacks in two genuine captures, all live Wi-Fi
// origins correlated to family flag 0 (including 118-, 114-, and 4-record
// responses), while every coordinate-neighborhood origin correlated to flag 1
// (all 400-record responses). RecordCount is therefore retained for diagnostics
// but deliberately not used to choose the consumer.
func CorrelateALSConsumerRoute(origin ALSQueryOrigin, summary ALSResponseFamilySummary) (ALSConsumerRoute, error) {
	if origin.OriginID < 0 || summary.OriginID < 0 {
		return ALSConsumerRoute{}, errors.New("ALS origin IDs must be non-negative")
	}
	if origin.OriginID != summary.OriginID {
		return ALSConsumerRoute{}, fmt.Errorf(
			"ALS response origin %d does not match request origin %d",
			summary.OriginID,
			origin.OriginID,
		)
	}
	if summary.RecordCount < 0 {
		return ALSConsumerRoute{}, errors.New("ALS response record count must be non-negative")
	}

	kind := strings.TrimSpace(strings.ToLower(origin.Kind))
	switch kind {
	case ALSQueryOriginLiveWifi:
		if origin.HasCoordinateCenter {
			return ALSConsumerRoute{}, errors.New("live Wi-Fi origin must not carry a coordinate center")
		}
		if strings.TrimSpace(origin.Reason) == "" {
			return ALSConsumerRoute{}, errors.New("live Wi-Fi origin requires a trace-visible reason")
		}
		if summary.FamilyFlag != ObservedALSResponseFamilyLive {
			return ALSConsumerRoute{}, fmt.Errorf(
				"live Wi-Fi origin requires observed response family flag %d",
				ObservedALSResponseFamilyLive,
			)
		}
		return ALSConsumerRoute{
			Origin:   origin,
			Summary:  summary,
			Consumer: "wifi-position-live",
			Method:   "trace-origin-family-correlation",
		}, nil

	case ALSQueryOriginCoordinateNeighborhood:
		if !origin.HasCoordinateCenter {
			return ALSConsumerRoute{}, errors.New("coordinate-neighborhood origin requires a coordinate center")
		}
		if summary.FamilyFlag != ObservedALSResponseFamilyBackground {
			return ALSConsumerRoute{}, fmt.Errorf(
				"coordinate-neighborhood origin requires observed response family flag %d",
				ObservedALSResponseFamilyBackground,
			)
		}
		return ALSConsumerRoute{
			Origin:   origin,
			Summary:  summary,
			Consumer: "background-neighborhood",
			Method:   "trace-origin-family-correlation",
		}, nil

	default:
		return ALSConsumerRoute{}, fmt.Errorf("unsupported ALS query origin kind %q", origin.Kind)
	}
}
