package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/controlapi"
	"github.com/ChrisMarxDev/tinkercloud/internal/llm"
)

type controlLLMValidator struct {
	mu        sync.Mutex
	providers []llm.Provider
	err       error
	models    map[llm.Provider][]string
	listErr   map[llm.Provider]error
	keys      []string
}

func (v *controlLLMValidator) Validate(_ context.Context, provider llm.Provider, _ []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.providers = append(v.providers, provider)
	return v.err
}

func (v *controlLLMValidator) ListModels(_ context.Context, provider llm.Provider, key []byte) ([]string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys = append(v.keys, string(key))
	if err := v.listErr[provider]; err != nil {
		return nil, err
	}
	return append([]string(nil), v.models[provider]...), nil
}

func llmProfileForm(connection string, revision uint64) controlapi.LLMProfileInput {
	return controlapi.LLMProfileInput{ConnectionID: connection, Model: "fixed-model", MaxMessages: 2, MaxMessageBytes: 20, MaxInputBytes: 40, MaxOutputTokens: 4, TimeoutMS: 1000, ViewerRequests: 1, AppRequests: 2, RateWindowMS: 1000, ConcurrencyLimit: 1, ExpectedRevision: revision}
}

func TestControlLLMCreateRotateProfileAndPolicyUseDerivedTargets(t *testing.T) {
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
	var isDefault bool
	if err := store.DB.QueryRow("SELECT id,is_default FROM llm_chat_profiles").Scan(&profile, &isDefault); err != nil || len(profile) != 32 || !isDefault {
		t.Fatalf("profile=%q default=%v err=%v", profile, isDefault, err)
	}
	limit := 20
	if err := service.UpdateAppLLMPolicy(context.Background(), operator, "alpha", "enabled", "specific", &limit, 0); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateAppLLMPolicy(context.Background(), operator, "alpha", "disabled", "inherit", nil, 0); !errors.Is(err, controlapi.ErrLLMRevision) {
		t.Fatalf("stale app policy=%v", err)
	}
	if err := service.UpdateAppLLMPolicy(context.Background(), operator, "missing", "enabled", "inherit", nil, 0); err == nil {
		t.Fatal("browser-selected missing app gained a policy")
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

func TestLLMDashboardOmitsProfileWithDisabledConnection(t *testing.T) {
	repo, store := llmRepo(t)
	defer store.Close()
	setupLLM(t, repo)
	if _, err := store.DB.Exec("UPDATE provider_connections SET status='disabled' WHERE id='conn'"); err != nil {
		t.Fatal(err)
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

func TestOperatorModelCatalogUsesOnlyActiveDecryptableSuccessfulConnections(t *testing.T) {
	repo, store := llmRepo(t)
	defer store.Close()
	setupLLM(t, repo)
	if err := repo.CreateConnection(context.Background(), LLMConnectionInput{ID: "gemini", DisplayName: "Gemini API key", Provider: llm.ProviderGemini, Secret: []byte("gemini-secret"), KeyVersion: 1, ActorID: "u"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateConnection(context.Background(), LLMConnectionInput{ID: "disabled", DisplayName: "Disabled API key", Provider: llm.ProviderAnthropic, Secret: []byte("disabled-secret"), KeyVersion: 1, ActorID: "u"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec("UPDATE provider_connections SET status='disabled' WHERE id='disabled'"); err != nil {
		t.Fatal(err)
	}
	validator := &controlLLMValidator{
		models:  map[llm.Provider][]string{llm.ProviderAnthropic: {"claude-current"}, llm.ProviderGemini: {"gemini-current"}},
		listErr: map[llm.Provider]error{llm.ProviderGemini: errors.New("provider unavailable")},
	}
	catalog, err := repo.OperatorModelCatalog(context.Background(), validator)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 1 || catalog[0].ConnectionID != "conn" || catalog[0].Model != "claude-current" || catalog[0].Provider != "anthropic" {
		t.Fatalf("unsafe or incomplete catalog: %#v", catalog)
	}
	keys := map[string]bool{}
	for _, key := range validator.keys {
		keys[key] = true
	}
	if len(validator.keys) != 2 || !keys["super-secret"] || !keys["gemini-secret"] || keys["disabled-secret"] {
		t.Fatalf("catalog did not use exactly the active stored keys: %#v", validator.keys)
	}
}

func TestOperatorModelCatalogOmitsUndecryptableAndInvalidProviderResults(t *testing.T) {
	repo, store := llmRepo(t)
	defer store.Close()
	setupLLM(t, repo)
	validator := &controlLLMValidator{models: map[llm.Provider][]string{llm.ProviderAnthropic: {"bad\nmodel"}}}
	catalog, err := repo.OperatorModelCatalog(context.Background(), validator)
	if err != nil || len(catalog) != 0 {
		t.Fatalf("invalid provider catalog survived: %#v, %v", catalog, err)
	}
	if _, err := store.DB.Exec("UPDATE provider_connections SET credential_envelope=X'01' WHERE id='conn'"); err != nil {
		t.Fatal(err)
	}
	validator = &controlLLMValidator{models: map[llm.Provider][]string{llm.ProviderAnthropic: {"claude-current"}}}
	catalog, err = repo.OperatorModelCatalog(context.Background(), validator)
	if err != nil || len(catalog) != 0 || len(validator.keys) != 0 {
		t.Fatalf("undecryptable connection reached provider or catalog: %#v keys=%#v err=%v", catalog, validator.keys, err)
	}
}
