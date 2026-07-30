package llm

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// Envelope keeps the host root outside SQLite. Production composition injects
// it from TinyHost's root-owned credential boundary; tests inject a fixed root.
type Envelope interface {
	Seal(context.Context, []byte) ([]byte, error)
	Open(context.Context, []byte) ([]byte, error)
}

var ErrEnvelope = errors.New("llm secret envelope unavailable")

type AESGCMEnvelope struct{ aead cipher.AEAD }

func NewAESGCMEnvelope(root []byte) (*AESGCMEnvelope, error) {
	if len(root) != 32 {
		return nil, ErrEnvelope
	}
	b, e := aes.NewCipher(root)
	if e != nil {
		return nil, ErrEnvelope
	}
	a, e := cipher.NewGCM(b)
	if e != nil {
		return nil, ErrEnvelope
	}
	return &AESGCMEnvelope{a}, nil
}
func (e *AESGCMEnvelope) Seal(ctx context.Context, secret []byte) ([]byte, error) {
	if e == nil || e.aead == nil || len(secret) == 0 {
		return nil, ErrEnvelope
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	n := make([]byte, e.aead.NonceSize())
	if _, x := io.ReadFull(rand.Reader, n); x != nil {
		return nil, ErrEnvelope
	}
	return append(n, e.aead.Seal(nil, n, secret, nil)...), nil
}
func (e *AESGCMEnvelope) Open(ctx context.Context, box []byte) ([]byte, error) {
	if e == nil || e.aead == nil || len(box) <= e.aead.NonceSize() {
		return nil, ErrEnvelope
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	p, x := e.aead.Open(nil, box[:e.aead.NonceSize()], box[e.aead.NonceSize():], nil)
	if x != nil {
		return nil, ErrEnvelope
	}
	return p, nil
}
