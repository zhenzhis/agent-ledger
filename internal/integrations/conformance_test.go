package integrations

import "testing"

func TestRunAdapterConformanceAutoDetectsProviderUsage(t *testing.T) {
	report, err := RunAdapterConformance("auto", []byte(`{
		"id":"resp_conf_1",
		"provider":"openai",
		"model":"gpt-5.5",
		"input":[{"role":"user","content":"must not persist"}],
		"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":25},"output_tokens":40},
		"metadata":{"agent_ledger.goal":"conformance provider","agent_ledger.project":"agent-ledger"}
	}`))
	if err != nil {
		t.Fatalf("conformance: %v", err)
	}
	if !report.OK || report.Status != "pass" || report.InputKind != "provider" || report.DecodedEvents != 2 || report.FailedEvents != 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
	for _, result := range report.Results {
		if result.EventID == "" || result.PayloadHash == "" || result.EventType == "" {
			t.Fatalf("result missing validation identity: %#v", result)
		}
	}
}

func TestRunAdapterConformanceReportsCanonicalPrivacyFailure(t *testing.T) {
	report, err := RunAdapterConformance("canonical", []byte(`{
		"source":"test-adapter",
		"event_type":"workload.started",
		"payload":{"goal":"safe","messages":[{"content":"must fail"}]}
	}`))
	if err != nil {
		t.Fatalf("conformance should return a report for validation failures: %v", err)
	}
	if report.OK || report.Status != "fail" || report.FailedEvents != 1 {
		t.Fatalf("expected failed report: %#v", report)
	}
	if len(report.Results) != 1 || report.Results[0].Error == "" {
		t.Fatalf("expected result error: %#v", report.Results)
	}
}

func TestRunAdapterConformanceCanonicalWarnings(t *testing.T) {
	report, err := RunAdapterConformance("canonical", []byte(`{
		"source":"test-adapter",
		"event_type":"workload.started",
		"payload":{"goal":"missing provenance"}
	}`))
	if err != nil {
		t.Fatalf("conformance: %v", err)
	}
	if !report.OK || report.Status != "pass_with_warnings" || report.WarningEvents != 1 || len(report.Recommendations) == 0 {
		t.Fatalf("expected provenance warning report: %#v", report)
	}
	strict, err := RunAdapterConformanceWithOptions(AdapterConformanceOptions{Kind: "canonical", Strict: true}, []byte(`{
		"source":"test-adapter",
		"event_type":"workload.started",
		"payload":{"goal":"missing provenance"}
	}`))
	if err != nil {
		t.Fatalf("strict conformance: %v", err)
	}
	if strict.OK || strict.Status != "fail" || strict.WarningEvents != 1 || strict.FailedEvents != 0 {
		t.Fatalf("expected strict warning failure: %#v", strict)
	}
}
