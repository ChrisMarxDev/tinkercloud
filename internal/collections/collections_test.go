package collections

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinyhost/tiny/internal/appauth"
)

func TestCollectionAndDocumentValidationGrammar(t *testing.T) {
	for _, value := range []string{"tasks", "a", "todo_items-2", "a" + strings.Repeat("b", 62) + "z"} {
		if !validCollection(value, 64) {
			t.Fatalf("valid collection rejected: %q", value)
		}
	}
	for _, value := range []string{"_tasks", "Tasks", "tasks/other", "task-", "a" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if validCollection(value, 64) {
			t.Fatalf("invalid collection accepted: %q", value)
		}
	}
	if !validID("doc_abcdefghijklmnopqrstuv") || validID("doc_short") {
		t.Fatal("document ID grammar mismatch")
	}
	for _, value := range []json.RawMessage{[]byte(`{}`), []byte(`{"name":"ok"}`)} {
		if !validDocument(value, 64<<10) {
			t.Fatalf("valid object rejected: %s", value)
		}
	}
	for _, value := range []json.RawMessage{[]byte(`[]`), []byte(`null`), []byte(`{`)} {
		if validDocument(value, 64<<10) {
			t.Fatalf("invalid document accepted: %s", value)
		}
	}
}

type noCallRepo struct{ called bool }

func (r *noCallRepo) Create(context.Context, string, string, Document) (Document, Mutation, error) {
	r.called = true
	return Document{}, Mutation{}, nil
}
func (r *noCallRepo) Get(context.Context, string, string, string) (*Document, error) {
	r.called = true
	return nil, nil
}
func (r *noCallRepo) Update(context.Context, string, string, string, json.RawMessage, *uint64) (Document, Mutation, error) {
	r.called = true
	return Document{}, Mutation{}, nil
}
func (r *noCallRepo) Delete(context.Context, string, string, string, *uint64) (bool, Mutation, error) {
	r.called = true
	return false, Mutation{}, nil
}
func (r *noCallRepo) List(context.Context, string, string, string, int) (ListResult, error) {
	r.called = true
	return ListResult{}, nil
}
func (r *noCallRepo) Snapshot(context.Context, string, string, int) (Snapshot, error) {
	r.called = true
	return Snapshot{}, nil
}

func TestServiceNeverTouchesRepositoryWithoutSealedAuthorization(t *testing.T) {
	repo := &noCallRepo{}
	service := New(repo, DefaultLimits())
	if _, _, err := service.Create(context.Background(), appauth.AuthorizationContext(nil), "tasks", json.RawMessage(`{}`)); err == nil {
		t.Fatal("nil authorization accepted")
	}
	if repo.called {
		t.Fatal("repository was reached without authorization")
	}
}
