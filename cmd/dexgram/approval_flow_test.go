package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"dexgram/internal/codex"
)

func TestRequestApprovalDispatchesCommandPermissionAndUserInput(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	app.bot = b

	commandParams, err := json.Marshal(approvalRequestParams{
		Command:                     "go test ./...",
		ProposedExecpolicyAmendment: []string{"allow go test"},
	})
	if err != nil {
		t.Fatalf("marshal command params: %v", err)
	}
	commandResult := make(chan any, 1)
	go func() {
		result, err := app.requestApproval(context.Background(), 123, 7, codex.ServerRequest{
			Method: "item/commandExecution/requestApproval",
			Params: commandParams,
		})
		if err != nil {
			t.Errorf("request command approval: %v", err)
			return
		}
		commandResult <- result
	}()
	waitPendingApproval(t, app).ch <- approvalDecision{result: "amend"}
	result := (<-commandResult).(map[string]any)
	decision := result["decision"].(map[string]any)
	if decision["acceptWithExecpolicyAmendment"] == nil {
		t.Fatalf("command approval result = %#v", result)
	}

	permissionsParams, err := json.Marshal(permissionRequestParams{
		Permissions: json.RawMessage(`{"sandbox":"workspace-write"}`),
	})
	if err != nil {
		t.Fatalf("marshal permission params: %v", err)
	}
	permissionResult := make(chan any, 1)
	go func() {
		result, err := app.requestApproval(context.Background(), 123, 7, codex.ServerRequest{
			Method: "item/permissions/requestApproval",
			Params: permissionsParams,
		})
		if err != nil {
			t.Errorf("request permission approval: %v", err)
			return
		}
		permissionResult <- result
	}()
	waitPendingApproval(t, app).ch <- approvalDecision{result: "permission-session"}
	permission := (<-permissionResult).(map[string]any)
	if permission["scope"] != "session" {
		t.Fatalf("permission result = %#v", permission)
	}

	emptyInput, err := app.requestApproval(context.Background(), 123, 7, codex.ServerRequest{
		Method: "item/tool/requestUserInput",
		Params: []byte(`{"questions":[]}`),
	})
	if err != nil {
		t.Fatalf("empty user input request: %v", err)
	}
	if emptyInput.(map[string]any)["answers"] == nil {
		t.Fatalf("empty user input result = %#v", emptyInput)
	}

	inputParams, err := json.Marshal(userInputRequestParams{Questions: []inputQuestion{{
		ID:       "choice",
		Question: "Pick one",
		Options:  []inputOption{{Label: "A"}, {Label: "B"}},
	}}})
	if err != nil {
		t.Fatalf("marshal input params: %v", err)
	}
	inputResult := make(chan any, 1)
	go func() {
		result, err := app.requestApproval(context.Background(), 123, 7, codex.ServerRequest{
			Method: "item/tool/requestUserInput",
			Params: inputParams,
		})
		if err != nil {
			t.Errorf("request user input: %v", err)
			return
		}
		inputResult <- result
	}()
	waitPendingInput(t, app).ch <- inputDecision{result: map[string]any{"choice": map[string]any{"answers": []string{"B"}}}}
	answers := (<-inputResult).(map[string]any)["answers"].(map[string]any)
	if answers["choice"].(map[string]any)["answers"].([]string)[0] != "B" {
		t.Fatalf("user input answers = %#v", answers)
	}
	if api.count("sendMessage") < 3 {
		t.Fatalf("sendMessage count = %d, want at least 3", api.count("sendMessage"))
	}
}

func TestRequestApprovalRejectsUnsupportedMethod(t *testing.T) {
	app := newHandlerTestApp(t, []int64{123})
	if _, err := app.requestApproval(context.Background(), 123, 7, codex.ServerRequest{Method: "unknown"}); err == nil {
		t.Fatal("expected unsupported approval request to fail")
	}
}

func waitPendingApproval(t *testing.T, app *app) *pendingApproval {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		app.mu.Lock()
		for _, pending := range app.approvals {
			app.mu.Unlock()
			return pending
		}
		app.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for pending approval")
	return nil
}

func waitPendingInput(t *testing.T, app *app) *pendingInput {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		app.mu.Lock()
		for _, pending := range app.inputs {
			app.mu.Unlock()
			return pending
		}
		app.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for pending input")
	return nil
}
