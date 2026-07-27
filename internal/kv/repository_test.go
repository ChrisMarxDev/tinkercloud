package kv

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type spyRepo struct{ apps []string }

func (s *spyRepo) Get(_ context.Context, a, k string) (*Entry, error) {
	s.apps = append(s.apps, a)
	return nil, nil
}
func (s *spyRepo) Set(_ context.Context, a, k string, v json.RawMessage, e *uint64) (Entry, error) {
	s.apps = append(s.apps, a)
	return Entry{Key: k, Value: v, Version: 1, UpdatedAt: time.Now()}, nil
}
func (s *spyRepo) Delete(_ context.Context, a, k string, e *uint64) (bool, Mutation, error) {
	s.apps = append(s.apps, a)
	return true, Mutation{Key: k, Deleted: true}, nil
}
func (s *spyRepo) List(_ context.Context, a, p, c string, l int) (ListResult, error) {
	s.apps = append(s.apps, a)
	return ListResult{}, nil
}
func TestServiceRepositoryReceivesOnlyAuthorizationApp(t *testing.T) {
	r := &spyRepo{}
	s := NewWithRepository(DefaultLimits(), nil, r)
	a := authorized(t, "server-app")
	_, _ = s.Get(context.Background(), a, "k")
	_, _ = s.Set(context.Background(), a, "k", json.RawMessage(`1`), nil)
	_, _ = s.Delete(context.Background(), a, "k", nil)
	_, _ = s.List(context.Background(), a, "", "", 1)
	for _, v := range r.apps {
		if v != "server-app" {
			t.Fatal(v)
		}
	}
}

type contextSpyRepo struct {
	getContext  context.Context
	listContext context.Context
	getApp      string
	listApp     string
}

func (s *contextSpyRepo) Get(ctx context.Context, app, _ string) (*Entry, error) {
	s.getContext, s.getApp = ctx, app
	return nil, ctx.Err()
}
func (s *contextSpyRepo) Set(context.Context, string, string, json.RawMessage, *uint64) (Entry, error) {
	panic("unexpected set")
}
func (s *contextSpyRepo) Delete(context.Context, string, string, *uint64) (bool, Mutation, error) {
	panic("unexpected delete")
}
func (s *contextSpyRepo) List(ctx context.Context, app, _, _ string, _ int) (ListResult, error) {
	s.listContext, s.listApp = ctx, app
	return ListResult{}, ctx.Err()
}

func TestServicePropagatesReadCancellationWithoutChangingScope(t *testing.T) {
	repo := &contextSpyRepo{}
	service := NewWithRepository(DefaultLimits(), nil, repo)
	auth := authorized(t, "server-app")

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Get(canceled, auth, "key"); !errors.Is(err, context.Canceled) {
		t.Fatalf("get cancellation: %v", err)
	}
	if repo.getContext != canceled || !errors.Is(repo.getContext.Err(), context.Canceled) {
		t.Fatal("get repository did not receive the canceled request context")
	}
	if repo.getApp != "server-app" {
		t.Fatalf("get scope changed: %q", repo.getApp)
	}

	deadline := time.Now().Add(time.Hour)
	bounded, stop := context.WithDeadline(context.Background(), deadline)
	defer stop()
	if _, err := service.List(bounded, auth, "", "", 1); err != nil {
		t.Fatalf("list with live deadline: %v", err)
	}
	gotDeadline, ok := repo.listContext.Deadline()
	if repo.listContext != bounded || !ok || !gotDeadline.Equal(deadline) {
		t.Fatal("list repository did not receive the request deadline context")
	}
	if repo.listApp != "server-app" {
		t.Fatalf("list scope changed: %q", repo.listApp)
	}
}
