// Package live owns V1's bounded in-memory, app-scoped notification hub.
// It is deliberately transport-neutral; the gateway authenticates before attach.
package live

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/capabilities"
	"github.com/tinyhost/tiny/internal/collections"
	"github.com/tinyhost/tiny/internal/kv"
)

var (
	ErrInvalidChannel = errors.New("invalid live channel")
	ErrLimit          = errors.New("live limit exceeded")
	ErrClosed         = errors.New("live connection closed")
)

type Limits struct{ ConnectionsPerApp, ConnectionsPerViewer, SubscriptionsPerConnection, PayloadBytes int }

// DefaultLimits deliberately keeps live frames below the KV value limit. Live
// is an ephemeral convenience channel, so one connection must never be able
// to reserve large amounts of memory or block a broadcaster for an unbounded
// time.
func DefaultLimits() Limits { return Limits{100, 5, 32, 32 << 10} }

// Transport is implemented by the gateway's WebSocket adapter. Send must be
// bounded/non-blocking; a slow consumer returns an error and is closed.
type Transport interface {
	Send(context.Context, Envelope) error
	Close(code int, reason string)
}
type Envelope struct {
	V          int             `json:"v"`
	Type       string          `json:"type"`
	Channel    string          `json:"channel,omitempty"`
	Event      string          `json:"event,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Key        string          `json:"key,omitempty"`
	ID         string          `json:"id,omitempty"`
	Collection string          `json:"collection,omitempty"`
	Version    uint64          `json:"version,omitempty"`
	Revision   uint64          `json:"revision,omitempty"`
	Deleted    bool            `json:"deleted,omitempty"`
}
type connection struct {
	auth                   appauth.AuthorizationContext
	app, identity, session string
	transport              Transport
	subscriptions          map[string]struct{}
	kvPrefixes             map[string]struct{}
	collections            map[string]struct{}
}
type Hub struct {
	mu          sync.Mutex
	limits      Limits
	connections map[*connection]struct{}
}

func New(l Limits) *Hub {
	if l.ConnectionsPerApp == 0 {
		l = DefaultLimits()
	}
	return &Hub{limits: l, connections: map[*connection]struct{}{}}
}
func validChannel(v string) bool {
	return v != "" && len([]byte(v)) <= 128 && !strings.HasPrefix(v, "_tiny") && !strings.ContainsRune(v, '\x00')
}
func validCollection(v string) bool {
	if len(v) < 1 || len(v) > 64 || !(v[0] >= 'a' && v[0] <= 'z' || v[0] >= '0' && v[0] <= '9') {
		return false
	}
	last := v[len(v)-1]
	if !(last >= 'a' && last <= 'z' || last >= '0' && last <= '9') {
		return false
	}
	for i := 1; i < len(v)-1; i++ {
		c := v[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func (h *Hub) Attach(auth appauth.AuthorizationContext, t Transport) (*Connection, error) {
	app, id, session, err := capabilities.Scope(auth)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrClosed
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	appCount, viewerCount := 0, 0
	for c := range h.connections {
		if c.app == app {
			appCount++
		}
		if c.app == app && c.identity == id {
			viewerCount++
		}
	}
	if appCount >= h.limits.ConnectionsPerApp || viewerCount >= h.limits.ConnectionsPerViewer {
		return nil, ErrLimit
	}
	c := &connection{auth: auth, app: app, identity: id, session: session, transport: t, subscriptions: map[string]struct{}{}, kvPrefixes: map[string]struct{}{}, collections: map[string]struct{}{}}
	h.connections[c] = struct{}{}
	return &Connection{hub: h, inner: c}, nil
}

type Connection struct {
	hub   *Hub
	inner *connection
}

func (c *Connection) Subscribe(channel string) error {
	if !validChannel(channel) {
		return ErrInvalidChannel
	}
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	if _, ok := c.hub.connections[c.inner]; !ok {
		return ErrClosed
	}
	if len(c.inner.subscriptions)+len(c.inner.kvPrefixes)+len(c.inner.collections) >= c.hub.limits.SubscriptionsPerConnection {
		return ErrLimit
	}
	c.inner.subscriptions[channel] = struct{}{}
	return nil
}
func (c *Connection) Unsubscribe(channel string) error {
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	if _, ok := c.hub.connections[c.inner]; !ok {
		return ErrClosed
	}
	delete(c.inner.subscriptions, channel)
	return nil
}
func (c *Connection) SubscribeKV(prefix string) error {
	if len([]byte(prefix)) > 256 || strings.ContainsRune(prefix, '\x00') {
		return ErrInvalidChannel
	}
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	if _, ok := c.hub.connections[c.inner]; !ok {
		return ErrClosed
	}
	if len(c.inner.subscriptions)+len(c.inner.kvPrefixes)+len(c.inner.collections) >= c.hub.limits.SubscriptionsPerConnection {
		return ErrLimit
	}
	c.inner.kvPrefixes[prefix] = struct{}{}
	return nil
}
func (c *Connection) UnsubscribeKV(prefix string) error {
	if len([]byte(prefix)) > 256 || strings.ContainsRune(prefix, '\x00') {
		return ErrInvalidChannel
	}
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	if _, ok := c.hub.connections[c.inner]; !ok {
		return ErrClosed
	}
	delete(c.inner.kvPrefixes, prefix)
	return nil
}
func (c *Connection) SubscribeCollection(collection string) error {
	if !validCollection(collection) {
		return ErrInvalidChannel
	}
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	if _, ok := c.hub.connections[c.inner]; !ok {
		return ErrClosed
	}
	if _, exists := c.inner.collections[collection]; exists {
		return nil
	}
	if len(c.inner.subscriptions)+len(c.inner.kvPrefixes)+len(c.inner.collections) >= c.hub.limits.SubscriptionsPerConnection {
		return ErrLimit
	}
	c.inner.collections[collection] = struct{}{}
	return nil
}
func (c *Connection) UnsubscribeCollection(collection string) error {
	if !validCollection(collection) {
		return ErrInvalidChannel
	}
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	if _, ok := c.hub.connections[c.inner]; !ok {
		return ErrClosed
	}
	delete(c.inner.collections, collection)
	return nil
}
func (c *Connection) Publish(ctx context.Context, channel, event string, payload json.RawMessage) error {
	if !validChannel(channel) || event == "" || len([]byte(event)) > 128 || !json.Valid(payload) || len(payload) > c.hub.limits.PayloadBytes {
		return ErrInvalidChannel
	}
	return c.hub.publishFrom(ctx, c.inner, channel, Envelope{V: 1, Type: "event", Channel: channel, Event: event, Payload: append(json.RawMessage(nil), payload...)})
}
func (c *Connection) Close() { c.hub.close(c.inner, 1000, "closed") }

// publishFrom checks the exact publisher while holding the same mutex used by
// Revoke/Close. A detached Connection object cannot use a buffered inbound
// frame to publish after revocation.
func (h *Hub) publishFrom(ctx context.Context, sender *connection, channel string, e Envelope) error {
	h.mu.Lock()
	if _, ok := h.connections[sender]; !ok {
		h.mu.Unlock()
		return ErrClosed
	}
	targets := make([]*connection, 0)
	for c := range h.connections {
		if c.app == sender.app {
			if _, ok := c.subscriptions[channel]; ok {
				targets = append(targets, c)
			}
		}
	}
	h.mu.Unlock()
	for _, c := range targets {
		if c.transport.Send(ctx, e) != nil {
			h.close(c, 1013, "slow consumer")
		}
	}
	return nil
}

// PublishKVChange satisfies kv.ChangeSink. It is called only after KV mutation.
func (h *Hub) PublishKVChange(ctx context.Context, auth appauth.DataAuthorizationContext, m kv.Mutation) {
	if auth == nil || auth.AppID() == "" {
		return
	}
	app := auth.AppID()
	e := Envelope{V: 1, Type: "kv.changed", Key: m.Key, Version: m.Version, Deleted: m.Deleted}
	h.mu.Lock()
	targets := make([]*connection, 0)
	for c := range h.connections {
		if c.app != app {
			continue
		}
		for prefix := range c.kvPrefixes {
			if strings.HasPrefix(m.Key, prefix) {
				targets = append(targets, c)
				break
			}
		}
	}
	h.mu.Unlock()
	for _, c := range targets {
		if c.transport.Send(ctx, e) != nil {
			h.close(c, 1013, "slow consumer")
		}
	}
}

// PublishCollectionChange satisfies collections.ChangeSink. It is called only
// after the app-local SQLite transaction commits, so it is a freshness hint
// rather than an uncommitted or durable event.
func (h *Hub) PublishCollectionChange(ctx context.Context, auth appauth.DataAuthorizationContext, m collections.Mutation) {
	if auth == nil || auth.AppID() == "" {
		return
	}
	app := auth.AppID()
	e := Envelope{V: 1, Type: "collection.changed", Collection: m.Collection, ID: m.ID, Version: m.Version, Revision: m.Revision, Deleted: m.Deleted}
	h.mu.Lock()
	targets := make([]*connection, 0)
	for c := range h.connections {
		if c.app == app {
			if _, ok := c.collections[m.Collection]; ok {
				targets = append(targets, c)
			}
		}
	}
	h.mu.Unlock()
	for _, c := range targets {
		if c.transport.Send(ctx, e) != nil {
			h.close(c, 1013, "slow consumer")
		}
	}
}

// Revoke closes every matching connection; callers pass only server-derived IDs.
func (h *Hub) Revoke(appID, sessionID string) {
	h.mu.Lock()
	targets := make([]*connection, 0)
	for c := range h.connections {
		if c.app == appID && (sessionID == "" || c.session == sessionID) {
			targets = append(targets, c)
		}
	}
	h.mu.Unlock()
	for _, c := range targets {
		h.close(c, 1008, "authorization revoked")
	}
}
func (h *Hub) close(c *connection, code int, reason string) {
	h.mu.Lock()
	_, ok := h.connections[c]
	if ok {
		delete(h.connections, c)
	}
	h.mu.Unlock()
	if ok {
		c.transport.Close(code, reason)
	}
}
