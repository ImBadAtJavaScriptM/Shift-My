package wloc

import (
	"errors"
	"fmt"
	"strings"
)

// ALSTransportRegistration models the trace-visible relationship between one
// locationd-created network task and the higher-level ALS query that owns it.
//
// TaskID is lab-local opaque text analogous to a CFNetwork task UUID. ActivityID
// is retained as an execution-context hint only; traces show that callback logs
// may temporarily lose the activity while the task itself remains associated
// with the query that created it.
//
// ParentRequesterToken is the requester token visible when the high-level query
// is issued. A later response may complete under a different child requester
// token while preserving IssuedSerial, so token equality is intentionally not
// required at completion.
type ALSTransportRegistration struct {
	TaskID               string
	ActivityID           uint64
	IssuedSerial         int
	ParentRequesterToken uint64
}

// ALSTransportCompletion exposes the controlled-lab result of a registered
// task receiving and parsing one WLOC response.
type ALSTransportCompletion struct {
	Registration        ALSTransportRegistration
	CompletionRequester ALSRequesterSnapshot
	ResponseVersion     uint16
	ResponseFunctionID  uint32
	ResponseRecords     int
}

// ALSTransportRegistry models only task ownership and response acceptance.
// It performs no networking and never calls private platform APIs.
//
// The important trace-backed property is that a response is accepted only in
// the context of a task that was previously created for an ALS query. A valid
// WLOC byte string delivered outside that task context is not enough.
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
		return errors.New("ALS issued serial must be non-negative")
	}
	if registration.ParentRequesterToken == 0 {
		return errors.New("ALS parent requester token is required")
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

// Complete parses one WLOC response inside a previously registered task
// context and then hands the decoded AP records to the normal completion
// dispatcher.
//
// IssuedSerial must match the registration, but RequesterToken may differ from
// ParentRequesterToken. That intentionally represents the observed fan-out in
// which one issued serial produces several child requester completions.
func (r *ALSTransportRegistry) Complete(taskID string, requester ALSRequesterSnapshot, responseBytes []byte) (ALSTransportCompletion, error) {
	key := normalizeALSTaskID(taskID)
	registration, ok := r.tasks[key]
	if !ok {
		return ALSTransportCompletion{}, errors.New("ALS response has no registered transport task")
	}
	if requester.RequesterToken == 0 {
		return ALSTransportCompletion{}, errors.New("ALS completion requester token is required")
	}
	if requester.IssuedSerial != registration.IssuedSerial {
		return ALSTransportCompletion{}, fmt.Errorf(
			"ALS completion issued serial %d does not match registered serial %d",
			requester.IssuedSerial,
			registration.IssuedSerial,
		)
	}

	version, functionID, devices, err := ParseResponse(responseBytes)
	if err != nil {
		return ALSTransportCompletion{}, fmt.Errorf("parse registered ALS response: %w", err)
	}
	if err := r.dispatcher.Complete(ALSCompletion{
		Requester: requester,
		Response:  devices,
	}); err != nil {
		return ALSTransportCompletion{}, err
	}

	// A CFNetwork task is one-shot for the observed WLOC transactions. Remove
	// the registration only after parsing and dispatch handoff both succeed.
	delete(r.tasks, key)

	return ALSTransportCompletion{
		Registration:        registration,
		CompletionRequester: requester,
		ResponseVersion:     version,
		ResponseFunctionID:  functionID,
		ResponseRecords:     len(devices),
	}, nil
}
