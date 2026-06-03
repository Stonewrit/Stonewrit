package spec

import (
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
)

func newValidator() *validator.Validate {
	return validator.New(validator.WithRequiredStructEnabled())
}

func validEvent() EventInput {
	return EventInput{
		EventType:  "data.accessed",
		OccurredAt: time.Now().UTC(),
		Source:     Source{System: "billing", Service: "api"},
		Actor:      Actor{Type: "human", Email: "u1@example.com"},
		Action:     Action{Name: "read", Category: "data", Result: "allowed"},
		Resource:   Resource{Type: "invoice"},
	}
}

func TestEventInput_ValidPasses(t *testing.T) {
	if err := newValidator().Struct(validEvent()); err != nil {
		t.Fatalf("a fully valid event should pass validation: %v", err)
	}
}

func TestEventInput_RejectsInvalid(t *testing.T) {
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}

	cases := []struct {
		name   string
		mutate func(*EventInput)
	}{
		{"missing_event_type", func(e *EventInput) { e.EventType = "" }},
		{"event_type_too_long", func(e *EventInput) { e.EventType = string(long) }},
		{"missing_source_system", func(e *EventInput) { e.Source.System = "" }},
		{"missing_actor_type", func(e *EventInput) { e.Actor.Type = "" }},
		{"bad_actor_email", func(e *EventInput) { e.Actor.Email = "not-an-email" }},
		{"missing_action_result", func(e *EventInput) { e.Action.Result = "" }},
		{"missing_resource_type", func(e *EventInput) { e.Resource.Type = "" }},
	}
	v := newValidator()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := validEvent()
			tc.mutate(&e)
			if err := v.Struct(e); err == nil {
				t.Errorf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestBatchRequest_Bounds(t *testing.T) {
	v := newValidator()
	if err := v.Struct(BatchRequest{Events: nil}); err == nil {
		t.Error("empty batch should fail the min=1 rule")
	}
	if err := v.Struct(BatchRequest{Events: []EventInput{validEvent()}}); err != nil {
		t.Errorf("a one-event batch should pass: %v", err)
	}
}
