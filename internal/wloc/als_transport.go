package wloc

import (
	"errors"
	"fmt"
	"strings"
)

// ALSTransportRegistration models one locationd-created CFNetwork task and the
// high-level WLOC origin that owns it.
//
// IssuedSerial is retained as the ALS serial visible when the task is created.
// It is a scheduling snapshot, not a strict task identity: genuine live and
// background tasks were observed completing under later issued serials while
// keeping the same CFNetwork TaskID and high-level OriginID.
//
// ParentRequesterToken is likewise an ownership hint. One requester can span
// several serials/tasks, and a response may finish under a child requester.
type ALSTransportRegistration struct {
	TaskID               string
	ActivityID           uint64
	Origin               ALSQueryOrigin
	IssuedSerial         int
	ParentRequesterToken uint64
}

// ALSTransportCompletion exposes the controlled-lab result of a registered
// task receiving and parsing one WLOC response.
type ALSTransportCompletion struct {
	Registration        ALSTransportRegistration
	CompletionRequester ALSRequesterSnapshot
	Route               ALSConsumerRoute
	ResponseVersion     uint16
	ResponseFunctionID  uint32
	ResponseRecords     int
	SerialAdvance       int
	DeliveredToWifi     bool
}

// ALSTransportRegistry models only task ownership, origin correlation and
// response routing. It performs no networking and never calls private platform
// APIs.
//
// A valid WLOC byte string is accepted only inside a task previously created
// for a high-level WLOC origin. The response-summary OriginID must match that
// registered origin. Live-family responses are handed to the WifiPosition lab
// dispatcher; background-neighborhood responses are parsed/correlated but are
// deliberately not injected into WifiPosition.
type ALSTransportRegistry struct {
	dispatcher *ALSCompletionDispatcher
	tasks      map[string]ALSTransportRegistration
}

func NewALSTransportRegistry(dispatcher *ALSCompletionDispatcher) *ALSTransportRegistry {
	if dispatcher == nil {
		dispatcher = NewALSCompletionDispatcher(nil)
	}
	return &ALSTransportRegistry{
		dispatcher: dispatcher,
		tasks:      make(map[string]ALSTransportRegistration),
	}
}

func normalizeALSTaskID(taskID string) string {
	return strings.TrimSpace(strings.ToLower(taskID))
}

func (r *ALSTransportRegistry) Register(registration ALSTransportRegistration) error {
	taskID := normalizeALSTaskID(registration.TaskID)
	if taskID == "" {
		return errors.New("ALS transport task ID is required")
	}
	if registration.IssuedSerial < 0 {
		return errors.New("ALS registration issued serial must be non-negative")
	}
	if registration.ParentRequesterToken == 0 {
		return errors.New("ALS parent requester token is required")
	}
	if registration.Origin.OriginID < 0 {
		return errors.New("ALS origin ID must be non-negative")
	}
	if strings.TrimSpace(registration.Origin.Kind) == "" {
		return errors.New("ALS query origin kind is required")
	}
	if _, exists := r.tasks[taskID]; exists {
		return errors.New("ALS transport task is already registered")
	}
	registration.TaskID = taskID
	r.tasks[taskID] = registration
	return nil
}

func (r *ALSTransportRegistry) PendingTasks() int {
	return len(r.tasks)
}

func (r *ALSTransportRegistry) Dispatcher() *ALSCompletionDispatcher {
	return r.dispatcher
}

// Complete correlates a response with a previously registered task and its
// high-level origin.
//
// The completion requester/serial is intentionally allowed to differ from the
// registration-time requester/serial. In the genuine traces the same CFNetwork
// task created beside one serial later completed under a newer serial. What
// remained stable was the TaskID plus the GeneralCLX OriginID.
//
// responseSummary is trace-side metadata in this lab model; it is not claimed
// to be part of the WLOC protobuf payload.
func (r *ALSTransportRegistry) Complete(
	taskID string,
	requester ALSRequesterSnapshot,
	responseSummary ALSResponseFamilySummary,
	responseBytes []byte,
) (ALSTransportCompletion, error) {
	key := normalizeALSTaskID(taskID)
	registration, ok := r.tasks[key]
	if !ok {
		return ALSTransportCompletion{}, errors.New("ALS response has no registered transport task")
	}
	if requester.RequesterToken == 0 {
		return ALSTransportCompletion{}, errors.New("ALS completion requester token is required")
	}
	if requester.IssuedSerial < 0 {
		return ALSTransportCompletion{}, errors.New("ALS completion issued serial must be non-negative")
	}

	route, err := CorrelateALSConsumerRoute(registration.Origin, responseSummary)
	if err != nil {
		return ALSTransportCompletion{}, fmt.Errorf("correlate ALS response origin: %w", err)
	}

	version, functionID, devices, err := ParseResponse(responseBytes)
	if err != nil {
		return ALSTransportCompletion{}, fmt.Errorf("parse registered ALS response: %w", err)
	}
	if responseSummary.RecordCount != len(devices) {
		return ALSTransportCompletion{}, fmt.Errorf(
			"ALS response summary records %d do not match decoded records %d",
			responseSummary.RecordCount,
			len(devices),
		)
	}

	deliveredToWifi := false
	if route.Consumer == "wifi-position-live" {
		if err := r.dispatcher.Complete(ALSCompletion{
			Requester: requester,
			Response:  devices,
		}); err != nil {
			return ALSTransportCompletion{}, err
		}
		deliveredToWifi = true
	}

	// One CFNetwork task represents one observed response transaction. Remove
	// the registration only after origin correlation, parsing, count validation
	// and any live WifiPosition handoff succeed.
	delete(r.tasks, key)

	return ALSTransportCompletion{
		Registration:        registration,
		CompletionRequester: requester,
		Route:               route,
		ResponseVersion:     version,
		ResponseFunctionID:  functionID,
		ResponseRecords:     len(devices),
		SerialAdvance:       requester.IssuedSerial - registration.IssuedSerial,
		DeliveredToWifi:     deliveredToWifi,
	}, nil
}
