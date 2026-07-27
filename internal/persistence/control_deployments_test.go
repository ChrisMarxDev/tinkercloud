package persistence

import (
	"bytes"
	"context"
	"github.com/tinyhost/tiny/internal/controlapi"
	"testing"
)

func TestControlCreateDeploymentNilServiceDenied(t *testing.T) {
	s := seeded(t)
	defer s.Close()
	c := ControlService{Store: s}
	if _, e := c.CreateDeployment(context.Background(), controlapi.Actor{ID: "u", Active: true}, "alpha", "k", controlapi.Upload{Reader: bytes.NewReader(nil), ContentType: "application/gzip"}); e == nil {
		t.Fatal("nil deployments accepted")
	}
}
