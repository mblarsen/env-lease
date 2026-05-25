package grantflow

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/lease"
)

func TestRunNonInteractiveMaterializesSimpleAndExplodedLeases(t *testing.T) {
	t.Setenv("ENV_LEASE_TEST", "1")

	var calls []materializeCall
	flow := Flow{Materialize: recordingMaterializer(&calls), Notice: func(string) {}}
	set := testSet(
		testEnvLease("mock-simple", "SIMPLE_KEY"),
		testExplodeLease("mock-explode"),
	)

	result, err := flow.Run(set, Options{Override: true})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !result.Request.Override {
		t.Fatal("expected override to be carried into grant request")
	}
	if result.Request.ConfigFile != set.ConfigFile {
		t.Fatalf("expected config file %q, got %q", set.ConfigFile, result.Request.ConfigFile)
	}

	gotVariables := variablesFromLeases(result.Request.Leases)
	wantVariables := []string{"SIMPLE_KEY", "", "KEY1", "KEY2"}
	if !reflect.DeepEqual(gotVariables, wantVariables) {
		t.Fatalf("variables mismatch\nwant: %#v\n got: %#v", wantVariables, gotVariables)
	}

	gotCalls := callVariablesAndValues(calls)
	wantCalls := []string{"SIMPLE_KEY=secret-for-mock-simple", "=", "KEY1=VALUE1", "KEY2=VALUE2"}
	if !reflect.DeepEqual(gotCalls, wantCalls) {
		t.Fatalf("materializer calls mismatch\nwant: %#v\n got: %#v", wantCalls, gotCalls)
	}
}

func TestRunInteractiveKeepsRoundOnePromptsBeforeExplodedChildren(t *testing.T) {
	t.Setenv("ENV_LEASE_TEST", "1")

	var prompts []string
	var calls []materializeCall
	flow := Flow{
		Confirm: func(prompt string) bool {
			prompts = append(prompts, prompt)
			return true
		},
		Materialize: recordingMaterializer(&calls),
	}
	set := testSet(
		testEnvLease("mock-simple", "SIMPLE_KEY"),
		testExplodeLease("mock-explode"),
	)

	result, err := flow.Run(set, Options{Interactive: true})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	wantPrompts := []string{
		"Grant 'SIMPLE_KEY'?",
		"Grant leases from 'mock-explode' (json, explode)?",
		"Grant lease for 'KEY1'?",
		"Grant lease for 'KEY2'?",
	}
	if !reflect.DeepEqual(prompts, wantPrompts) {
		t.Fatalf("prompts mismatch\nwant: %#v\n got: %#v", wantPrompts, prompts)
	}

	gotVariables := variablesFromLeases(result.Request.Leases)
	wantVariables := []string{"", "SIMPLE_KEY", "KEY1", "KEY2"}
	if !reflect.DeepEqual(gotVariables, wantVariables) {
		t.Fatalf("variables mismatch\nwant: %#v\n got: %#v", wantVariables, gotVariables)
	}
}

func TestRunInteractiveNoSelectionIsNoop(t *testing.T) {
	t.Setenv("ENV_LEASE_TEST", "1")

	var notices []string
	var calls []materializeCall
	flow := Flow{
		Confirm:     func(string) bool { return false },
		Notice:      func(message string) { notices = append(notices, message) },
		Materialize: recordingMaterializer(&calls),
	}

	result, err := flow.Run(testSet(testEnvLease("mock-simple", "SIMPLE_KEY")), Options{Interactive: true})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Noop {
		t.Fatal("expected no-op result")
	}
	if len(calls) != 0 {
		t.Fatalf("expected no materializer calls, got %d", len(calls))
	}
	if !reflect.DeepEqual(notices, []string{"No leases selected."}) {
		t.Fatalf("unexpected notices: %#v", notices)
	}
}

func TestRunInteractiveContinueOnLookupErrorReturnsSuccessfulGrant(t *testing.T) {
	t.Setenv("ENV_LEASE_TEST", "1")

	var calls []materializeCall
	flow := Flow{
		Confirm:     func(string) bool { return true },
		Materialize: recordingMaterializer(&calls),
	}
	set := testSet(
		testEnvLease("mock-fail", "FAIL_KEY"),
		testEnvLease("mock", "SUCCESS_KEY"),
	)

	result, err := flow.Run(set, Options{Interactive: true, ContinueOnError: true})
	if err != nil {
		t.Fatalf("expected successful partial grant result, got %v", err)
	}

	gotVariables := variablesFromLeases(result.Request.Leases)
	wantVariables := []string{"SUCCESS_KEY"}
	if !reflect.DeepEqual(gotVariables, wantVariables) {
		t.Fatalf("variables mismatch\nwant: %#v\n got: %#v", wantVariables, gotVariables)
	}

	gotCalls := callVariablesAndValues(calls)
	wantCalls := []string{"SUCCESS_KEY=secret-for-mock"}
	if !reflect.DeepEqual(gotCalls, wantCalls) {
		t.Fatalf("materializer calls mismatch\nwant: %#v\n got: %#v", wantCalls, gotCalls)
	}
}

func TestRunNonInteractiveContinueOnLookupErrorMaterializesSuccessfulLeases(t *testing.T) {
	t.Setenv("ENV_LEASE_TEST", "1")

	var calls []materializeCall
	flow := Flow{Materialize: recordingMaterializer(&calls)}
	set := testSet(
		testEnvLease("mock-fail", "FAIL_KEY"),
		testEnvLease("mock", "SUCCESS_KEY"),
	)

	_, err := flow.Run(set, Options{ContinueOnError: true})
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if !strings.Contains(err.Error(), "Lease: mock-fail") {
		t.Fatalf("expected mock-fail in error, got %v", err)
	}

	gotCalls := callVariablesAndValues(calls)
	wantCalls := []string{"SUCCESS_KEY=secret-for-mock"}
	if !reflect.DeepEqual(gotCalls, wantCalls) {
		t.Fatalf("materializer calls mismatch\nwant: %#v\n got: %#v", wantCalls, gotCalls)
	}
}

type materializeCall struct {
	Lease lease.Lease
	Value string
}

func recordingMaterializer(calls *[]materializeCall) MaterializeFunc {
	return func(l lease.Lease, secret string) (Materialized, error) {
		*calls = append(*calls, materializeCall{Lease: l, Value: secret})
		return Materialized{Leases: []ipc.Lease{l.ToIPC()}}, nil
	}
}

func testSet(leases ...lease.Lease) *lease.Set {
	return &lease.Set{
		Root:       "/tmp/project",
		ConfigFile: "/tmp/project/env-lease.toml",
		Leases:     leases,
	}
}

func testEnvLease(source, variable string) lease.Lease {
	return lease.Lease{
		Provider:    "1password",
		Source:      source,
		Destination: "/tmp/project/.env",
		Duration:    "1m",
		LeaseType:   lease.TypeEnv,
		Variable:    variable,
		Format:      "%s=%q",
	}
}

func testExplodeLease(source string) lease.Lease {
	return lease.Lease{
		Provider:    "1password",
		Source:      source,
		Destination: "/tmp/project/.env",
		Duration:    "1m",
		LeaseType:   lease.TypeEnv,
		Transform:   []string{"json", "explode"},
		Format:      "%s=%q",
	}
}

func variablesFromLeases(leases []ipc.Lease) []string {
	variables := make([]string, 0, len(leases))
	for _, l := range leases {
		variables = append(variables, l.Variable)
	}
	return variables
}

func callVariablesAndValues(calls []materializeCall) []string {
	values := make([]string, 0, len(calls))
	for _, call := range calls {
		values = append(values, call.Lease.Variable+"="+call.Value)
	}
	return values
}
