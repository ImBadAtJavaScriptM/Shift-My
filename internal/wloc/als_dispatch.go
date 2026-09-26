package wloc

import (
	"errors"
	"sort"
	"strings"
)

// ALSRequesterSnapshot captures the stable correlation fields observed in
// locationd's private ALS log tuple. The field names IssuedSerial and
// CompletedSerial are descriptive names derived from their monotonic behavior;
// they are not claimed to be Apple's official names.
type ALSRequesterSnapshot struct {
	RequesterToken  uint64
	ProviderCode    int
	IssuedSerial    int
	CompletedSerial int
	Lane            int
}

// SerialGapHint returns the observed issued-minus-completed serial gap.
//
// Trace analysis shows IssuedSerial advances with high-level ALS query calls,
// while CompletedSerial advances with requester completions. One issued serial
// can fan out to several requester tokens, so this gap must not be interpreted
// as a literal count of pending network requests.
func (s ALSRequesterSnapshot) SerialGapHint() int {
	return s.IssuedSerial - s.CompletedSerial
}

// ALSCompletion models one parsed network response reaching the shared
// AP-location service. Multiple completions may be coalesced before the Wi-Fi
// provider is asked to re-evaluate its current scan.
type ALSCompletion struct {
	Requester ALSRequesterSnapshot
	Response  []DeviceLocation
}

// ALSDispatchResult represents one coalesced Network::AlsFinished-style
// re-evaluation in the controlled lab.
type ALSDispatchResult struct {
	Event                string
	Cause                string
	CoalescedCompletions int
	Requesters           []ALSRequesterSnapshot
	CachedLocations      int
	Outcome              ALSLifecycleOutcome
}

// ALSAccessPointLocationService is a controlled, in-memory approximation of the
// AP-location service layer suggested by locationd traces and public symbols.
// It stores parsed AP locations by BSSID and intentionally does not perform any
// networking or call private platform APIs.
type ALSAccessPointLocationService struct {
	locations map[string]DeviceLocation
}

func NewALSAccessPointLocationService() *ALSAccessPointLocationService {
	return &ALSAccessPointLocationService{
		locations: make(map[string]DeviceLocation),
	}
}

// MergeResponse incorporates one parsed WLOC response. For duplicate BSSIDs,
// a usable location beats a missing location and a smaller positive horizontal
// accuracy beats a larger one.
func (s *ALSAccessPointLocationService) MergeResponse(response []DeviceLocation) {
	if s.locations == nil {
		s.locations = make(map[string]DeviceLocation)
	}
	for _, device := range response {
		if !looksLikeBSSID(device.BSSID) {
			continue
		}
		key := strings.ToLower(device.BSSID)
		current, ok := s.locations[key]
		if !ok || preferCachedLocation(device, current) {
			s.locations[key] = device
		}
	}
}

func preferCachedLocation(candidate, current DeviceLocation) bool {
	candidateUsable := usableWifiLocation(candidate)
	currentUsable := usableWifiLocation(current)
	if candidateUsable != currentUsable {
		return candidateUsable
	}
	if candidateUsable {
		if candidate.HorizontalAccuracy > 0 && current.HorizontalAccuracy > 0 {
			return candidate.HorizontalAccuracy < current.HorizontalAccuracy
		}
		if candidate.HorizontalAccuracy > 0 && current.HorizontalAccuracy <= 0 {
			return true
		}
	}
	// With equivalent usability/accuracy, keep the existing record so repeated
	// completions remain deterministic.
	return false
}

func (s *ALSAccessPointLocationService) Snapshot() []DeviceLocation {
	out := make([]DeviceLocation, 0, len(s.locations))
	for _, device := range s.locations {
		out = append(out, device)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].BSSID) < strings.ToLower(out[j].BSSID)
	})
	return out
}

func (s *ALSAccessPointLocationService) Len() int {
	return len(s.locations)
}

// ALSCompletionDispatcher models the trace-backed asynchronous boundary:
// requesterDidFinish callbacks update the shared AP-location service and mark
// Wi-Fi state dirty. A later Flush coalesces one or more completions into a
// single Network::AlsFinished-style scan re-evaluation.
type ALSCompletionDispatcher struct {
	service       *ALSAccessPointLocationService
	pending       []ALSRequesterSnapshot
	hasDispatched bool
}

func NewALSCompletionDispatcher(service *ALSAccessPointLocationService) *ALSCompletionDispatcher {
	if service == nil {
		service = NewALSAccessPointLocationService()
	}
	return &ALSCompletionDispatcher{service: service}
}

func (d *ALSCompletionDispatcher) Complete(completion ALSCompletion) error {
	if completion.Requester.RequesterToken == 0 {
		return errors.New("requester token is required")
	}
	if completion.Requester.IssuedSerial < 0 || completion.Requester.CompletedSerial < 0 {
		return errors.New("requester serials must be non-negative")
	}
	d.service.MergeResponse(completion.Response)
	d.pending = append(d.pending, completion.Requester)
	return nil
}

func (d *ALSCompletionDispatcher) PendingCompletions() int {
	return len(d.pending)
}

func (d *ALSCompletionDispatcher) Service() *ALSAccessPointLocationService {
	return d.service
}

// Flush models the provider's deferred/coalesced re-evaluation. Calling Flush
// without a pending completion is rejected so the lab cannot accidentally
// pretend a private Network::AlsFinished event occurred spontaneously.
func (d *ALSCompletionDispatcher) Flush(scan []ScanObservation, maxAPs int) (ALSDispatchResult, error) {
	if len(d.pending) == 0 {
		return ALSDispatchResult{}, errors.New("no completed ALS requester is pending")
	}

	requesters := append([]ALSRequesterSnapshot(nil), d.pending...)
	d.pending = nil

	outcome, err := EvaluateALSLifecycle(scan, d.service.Snapshot(), maxAPs)
	if err != nil {
		return ALSDispatchResult{}, err
	}

	d.hasDispatched = true
	return ALSDispatchResult{
		Event:                "Network::AlsFinished",
		Cause:                "completed-requester",
		CoalescedCompletions: len(requesters),
		Requesters:           requesters,
		CachedLocations:      d.service.Len(),
		Outcome:              outcome,
	}, nil
}

// ReevaluateCached models the repeated Network::AlsFinished passes observed
// after the first completion-triggered dispatch. The trace shows the same
// cached ALS result set being rematched against the current scan without a new
// requesterDidFinish immediately beforehand.
//
// Re-evaluation is deliberately blocked while new completions are pending, and
// before any completion-triggered dispatch has primed the cache.
func (d *ALSCompletionDispatcher) ReevaluateCached(scan []ScanObservation, maxAPs int) (ALSDispatchResult, error) {
	if len(d.pending) != 0 {
		return ALSDispatchResult{}, errors.New("pending ALS completions must be flushed before cached re-evaluation")
	}
	if !d.hasDispatched {
		return ALSDispatchResult{}, errors.New("ALS cache has not completed an initial dispatch")
	}
	if d.service.Len() == 0 {
		return ALSDispatchResult{}, errors.New("ALS cache is empty")
	}

	outcome, err := EvaluateALSLifecycle(scan, d.service.Snapshot(), maxAPs)
	if err != nil {
		return ALSDispatchResult{}, err
	}
	return ALSDispatchResult{
		Event:                "Network::AlsFinished",
		Cause:                "cached-reevaluation",
		CoalescedCompletions: 0,
		CachedLocations:      d.service.Len(),
		Outcome:              outcome,
	}, nil
}
