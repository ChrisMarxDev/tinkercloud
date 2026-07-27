// Package audit defines the append-only evidence required by control mutations.
package audit

import (
	"context"
	"errors"
	"time"
)

var ErrUnavailable = errors.New("audit store unavailable")

type Event struct {
	ID, ActorKind, ActorID, AppID, Action, Outcome, TargetKind, TargetID, RequestID string
	OccurredAt                                                                      time.Time
	// Metadata must contain only allowlisted, non-secret values.
	Metadata map[string]string
}

type Recorder interface {
	Append(context.Context, Event) error
}

// TransactionalRecorder is implemented by persistence adapters that can append
// audit evidence in the same transaction as the protected mutation.
type TransactionalRecorder interface{ Recorder }
