package live

import (
	"encoding/json"
	"errors"
)

var ErrInvalidFrame = errors.New("invalid live frame")

// ClientFrame is transport-independent. The gateway WebSocket adapter decodes
// bytes then validates before calling Connection; no RFC6455 implementation is
// included in the domain package.
type ClientFrame struct {
	V       int             `json:"v"`
	Type    string          `json:"type"`
	Channel string          `json:"channel,omitempty"`
	Event   string          `json:"event,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Prefix  string          `json:"prefix,omitempty"`
}

func ParseClientFrame(raw []byte, payloadLimit int) (ClientFrame, error) {
	var f ClientFrame
	if len(raw) == 0 || len(raw) > payloadLimit || json.Unmarshal(raw, &f) != nil || f.V != 1 {
		return ClientFrame{}, ErrInvalidFrame
	}
	switch f.Type {
	case "subscribe", "unsubscribe":
		if !validChannel(f.Channel) {
			return ClientFrame{}, ErrInvalidFrame
		}
	case "subscribe_kv":
		if len(f.Prefix) > 256 {
			return ClientFrame{}, ErrInvalidFrame
		}
	case "publish":
		if !validChannel(f.Channel) || f.Event == "" || len(f.Event) > 128 || !json.Valid(f.Payload) || len(f.Payload) > payloadLimit {
			return ClientFrame{}, ErrInvalidFrame
		}
	default:
		return ClientFrame{}, ErrInvalidFrame
	}
	return f, nil
}
