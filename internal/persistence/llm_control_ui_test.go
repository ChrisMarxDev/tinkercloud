package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"github.com/ChrisMarxDev/tinkercloud/internal/deployments"
	"github.com/ChrisMarxDev/tinkercloud/internal/llm"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
)

type controlLLMValidator struct {
	providers []llm.Provider
	err       error
}

func (v *controlLLMValidator) Validate(_ context.Context, provider llm.Provider, _ []byte) error {
	v.providers = append(v.providers, provider)
	return v.err
}

func llmProfileForm(connection string, revision uint64) controlapi.LLMProfileInput {
	return controlapi.LLMProfileInput{ConnectionID: connection, Model: "fixed-model", MaxMessages: 2, MaxMessageBytes: 20, MaxInputBytes: 40, MaxOutputTokens: 4, TimeoutMS: 1000, ViewerRequests: 1, AppRequests: 2, RateWindowMS: 1000, ConcurrencyLimit: 1, MonthlyTokenLimit: 20, ExpectedRevision: revision}
}

func TestControlLLMCreateRotateProfileAndGrantUseDerivedTargets(t *testing.T) {
	repo, store := llmRepo(t)
	defer store.Close()
	validator := &controlLLMValidator{}
	service := ControlService{Store: store, LLM: &repo, LLMValidator: validator}
	operator := controlapi.Actor{ID: "u", Role: "operator", Active: true}
	if err := service.CreateLLMConnection(context.Background(), operator, "anthropic", "first-key"); err != nil {
		t.Fatal(err)
	}
	var id, provider, label string
	if err := store.DB.QueryRow("SELECT id,provider_kind,display_name FROM provider_connections").Scan(&id, &provider, &label); err != nil || len(id) != 32 || provider != "anthropic" || label != "Anthropic API key" {
		t.Fatalf("connection=%q provider=%q label=%q err=%v", id, provider, label, err)
	}
	// Rotate receives no browser provider. The stored provider is resolved and
	// used for validation before the credential envelope is replaced.
	if err := service.RotateLLMConnection(context.Background(), operator, id, "replacement-key"); err != nil {
		t.Fatal(err)
	}
	if len(validator.providers) != 2 || validator.providers[0] != llm.ProviderAnthropic || validator.providers[1] != llm.ProviderAnthropic {
		t.Fatalf("validator providers=%v", validator.providers)
	}
	if err := service.CreateLLMProfile(context.Background(), operator, llmProfileForm(id, 0)); err != nil {
		t.Fatal(err)
	}
	var profile string
	if err := store.DB.QueryRow("SELECT id FROM llm_chat_profiles").Scan(&profile); err != nil || len(profile) != 32 {
		t.Fatalf("profile=%q err=%v", profile, err)
	}
	if err := service.ApproveLLMGrant(context.Background(), operator, "alpha", profile, 0); err != nil {
		t.Fatal(err)
	}
	if err := service.SetLLMGrantStatus(context.Background(), operator, "alpha", "revoked", 1); err != nil {
		t.Fatal(err)
	}
	if err := service.SetLLMGrantStatus(context.Background(), operator, "alpha", "disabled", 1); !errors.Is(err, controlapi.ErrLLMRevision) {
		t.Fatalf("stale grant=%v", err)
	}
	if err := service.ApproveLLMGrant(context.Background(), operator, "missing", profile, 0); err == nil {
		t.Fatal("browser-selected missing app gained a grant")
	}
	if err := service.CreateLLMConnection(context.Background(), controlapi.Actor{ID: "u", Role: "deployer", Active: true}, "anthropic", "key"); err == nil {
		t.Fatal("deployer created an operator connection")
	}
}

func TestControlLLMCreateDerivesFixedLabelsAndValidationFailurePreservesExistingConnections(t *testing.T) {
	repo, store := llmRepo(t)
	defer store.Close()
	operator := controlapi.Actor{ID: "u", Role: "operator", Active: true}
	validator := &controlLLMValidator{}
	service := ControlService{Store: store, LLM: &repo, LLMValidator: validator}
	if err := service.CreateLLMConnection(context.Background(), operator, "gemini", "gemini-key"); err != nil {
		t.Fatal(err)
	}
	var label string
	if err := store.DB.QueryRow("SELECT display_name FROM provider_connections").Scan(&label); err != nil || label != "Gemini API key" {
		t.Fatalf("derived label=%q err=%v", label, err)
	}
	validator.err = errors.New("provider denied")
	if err := service.CreateLLMConnection(context.Background(), operator, "anthropic", "bad-key"); err == nil {
		t.Fatal("validation failure created a connection")
	}
	var count int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM provider_connections").Scan(&count); err != nil || count != 1 {
		t.Fatalf("validation failure changed connections=%d err=%v", count, err)
	}
}

func TestControlLLMKeyMutationsDenyBeforeProviderValidationWithoutEnvelope(t *testing.T) {
	repo, store := llmRepo(t)
	defer store.Close()
	repo.Envelope = nil
	validator := &controlLLMValidator{}
	service := ControlService{Store: store, LLM: &repo, LLMValidator: validator}
	operator := controlapi.Actor{ID: "u", Role: "operator", Active: true}
	if err := service.CreateLLMConnection(context.Background(), operator, "anthropic", "key"); err == nil {
		t.Fatal("missing envelope created a connection")
	}
	if len(validator.providers) != 0 {
		t.Fatalf("missing envelope invoked provider validation: %v", validator.providers)
	}
	withConnection, connectedStore := llmRepo(t)
	defer connectedStore.Close()
	setupLLM(t, withConnection)
	withConnection.Envelope = nil
	validator = &controlLLMValidator{}
	service = ControlService{Store: connectedStore, LLM: &withConnection, LLMValidator: validator}
	if err := service.RotateLLMConnection(context.Background(), operator, "conn", "replacement"); err == nil {
		t.Fatal("missing envelope rotated a connection")
	}
	if len(validator.providers) != 0 {
		t.Fatalf("missing envelope invoked provider validation during rotation: %v", validator.providers)
	}
}

func TestControlLLMProfileRevisionAndConnectionStateFailClosed(t *testing.T) {
	repo, store := llmRepo(t)
	defer store.Close()
	setupLLM(t, repo)
	service := ControlService{Store: store, LLM: &repo}
	operator := controlapi.Actor{ID: "u", Role: "operator", Active: true}
	if err := service.UpdateLLMProfile(context.Background(), operator, "profile", llmProfileForm("conn", 2)); !errors.Is(err, controlapi.ErrLLMRevision) {
		t.Fatalf("stale profile=%v", err)
	}
	if _, err := store.DB.Exec("UPDATE provider_connections SET status='disabled' WHERE id='conn'"); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateLLMProfile(context.Background(), operator, "profile", llmProfileForm("conn", 1)); !errors.Is(err, controlapi.ErrLLMRevision) {
		t.Fatalf("disabled connection profile update=%v", err)
	}
}

func TestLLMGrantRejectsDisabledConnectionAndDashboardOmitsUnselectableProfile(t *testing.T) {
	repo, store := llmRepo(t)
	defer store.Close()
	setupLLM(t, repo)
	if _, err := store.DB.Exec("UPDATE provider_connections SET status='disabled' WHERE id='conn'"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateGrant(context.Background(), LLMGrantInput{AppID: "a", ProfileID: "profile", OperatorID: "u", Status: "disabled"}, 1); !errors.Is(err, controlapi.ErrLLMRevision) {
		t.Fatalf("disabled connection grant update=%v", err)
	}
	if _, err := store.DB.Exec("INSERT INTO users(id,normalized_email,role,status,created_at) VALUES('op','operator@example.test','operator','active',datetime('now')); INSERT INTO access_policies(app_id,revision,mode,created_at) VALUES('a',1,'private',datetime('now')),('b',1,'private',datetime('now'))"); err != nil {
		t.Fatal(err)
	}
	v, err := (ControlService{Store: store, LLM: &repo}).Dashboard(context.Background(), controlapi.Actor{ID: "op", Role: "operator", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.LLMProfiles) != 0 {
		t.Fatalf("disabled connection profile remained selectable: %#v", v.LLMProfiles)
	}
	if v.LLMKeyManagementReady {
		t.Fatal("dashboard claimed key management ready without a validator")
	}
	v, err = (ControlService{Store: store, LLM: &repo, LLMValidator: &controlLLMValidator{}}).Dashboard(context.Background(), controlapi.Actor{ID: "op", Role: "operator", Active: true})
	if err != nil || !v.LLMKeyManagementReady {
		t.Fatalf("dashboard readiness=%v err=%v", v.LLMKeyManagementReady, err)
	}
}

func TestLLMActivationGateReadsCurrentProductionGrantAndConnectionState(t *testing.T) {
	for name, mutate := range map[string]func(*SQLiteStore){
		"disabled grant": func(s *SQLiteStore) {
			_, _ = s.DB.Exec("UPDATE app_capability_grants SET status='disabled' WHERE app_id='a'")
		},
		"revoked grant": func(s *SQLiteStore) {
			_, _ = s.DB.Exec("UPDATE app_capability_grants SET status='revoked' WHERE app_id='a'")
		},
		"disabled connection": func(s *SQLiteStore) {
			_, _ = s.DB.Exec("UPDATE provider_connections SET status='disabled' WHERE id='conn'")
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo, store := llmRepo(t)
			defer store.Close()
			setupLLM(t, repo)
			mutate(store)
			memory := &deployments.MemoryRepository{Records: map[string]deployments.Record{}, Current: map[string]string{}}
			old := deployments.Record{Deployment: releases.Deployment{ID: "old", AppID: "a", State: releases.Active}, OwnerID: "u", Manifest: releases.Manifest{Name: "alpha"}}
			next := deployments.Record{Deployment: releases.Deployment{ID: "next", AppID: "a", State: releases.Verified}, OwnerID: "u", Manifest: releases.Manifest{Name: "alpha", LLMChat: true}}
			memory.Records[old.ID], memory.Records[next.ID], memory.Current["a"] = old, next, old.ID
			service := &deployments.Service{Repo: memory, Gates: deployments.GateFuncs{PolicyFunc: func(context.Context, deployments.Record) bool { return true }, CertificateFunc: func(context.Context, deployments.Record) bool { return true }, ProbeFunc: func(context.Context, deployments.Record) bool { return true }}, CapabilityReady: func(ctx context.Context, r deployments.Record) bool { return repo.Available(ctx, r.AppID) }}
			if err := service.Activate(context.Background(), deployments.Actor{ID: "u", Active: true}, "next", "request"); err == nil {
				t.Fatal("inactive LLM binding activated deployment")
			}
			if memory.Current["a"] != "old" || memory.Records["next"].State != releases.Verified {
				t.Fatalf("activation pointer mutated: %#v", memory)
			}
		})
	}
}
