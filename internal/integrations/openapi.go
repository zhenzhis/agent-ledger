package integrations

import "github.com/zhenzhis/agent-ledger/internal/storage"

// OpenAPISpecFor returns a compact OpenAPI 3.1 description for the stable
// metadata-only control-plane surfaces. It intentionally describes contracts
// and envelope shapes instead of local files, prompt content, or secrets.
func OpenAPISpecFor(opts Options, runtime *storage.RuntimeStatus) map[string]interface{} {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	catalog := Registry(opts)
	discovery := Discovery(opts)
	return map[string]interface{}{
		"openapi": "3.1.0",
		"info": map[string]interface{}{
			"title":       "Agent Ledger Control Plane API",
			"summary":     "Local-first metadata-only AgentOps and FinOps control-plane API.",
			"description": "Stable REST contract surfaces for discovery, canonical events, adapter conformance, workload state, and runtime probes. Prompt and response content are outside this API contract.",
			"version":     "v1",
		},
		"servers": []map[string]string{
			{"url": "/", "description": "Same-origin local Agent Ledger server"},
		},
		"tags": []map[string]string{
			{"name": "contracts", "description": "Discovery, contract bundle, OpenAPI, runtime, and capability metadata"},
			{"name": "canonical-events", "description": "Metadata-only canonical event schema, validation, and ingest"},
			{"name": "adapter-conformance", "description": "Adapter contract and dry-run fixture validation"},
			{"name": "workload-feed", "description": "Cursor-stable workload state feed for local monitors and routers"},
		},
		"x-agent-ledger": map[string]interface{}{
			"contract":                "agent-ledger.control-plane-openapi",
			"version":                 "v1",
			"local_first":             true,
			"privacy_default":         catalog.PrivacyDefault,
			"read_only":               opts.ReadOnly,
			"prompt_content_stored":   false,
			"usage_data_uploaded":     false,
			"discovery_hash":          hashJSONPayload(discovery),
			"capability_catalog_hash": CatalogFingerprintFrom(catalog),
			"runtime_status_hash":     hashJSONPayload(runtime),
			"canonical_schema_hash":   storage.CanonicalEventSchemaFingerprint(),
			"adapter_spec_hash":       AdapterContractFingerprint(),
		},
		"paths": map[string]interface{}{
			"/.well-known/agent-ledger.json": getOperation("contracts", "Get discovery manifest", "Privacy-safe local discovery manifest.", "DiscoveryManifest"),
			"/api/discovery":                 getOperation("contracts", "Get discovery manifest", "Same discovery manifest under the API namespace.", "DiscoveryManifest"),
			"/api/contracts":                 getOperation("contracts", "Get contract bundle", "One-shot contract index with document hashes, revalidation semantics, CLI commands, and MCP entrypoints.", "ContractBundle"),
			"/api/contracts/verify":          getOperation("contracts", "Verify control-plane contracts", "Machine-readable self-check for discovery, contract bundle, OpenAPI, schema, adapter, runtime, and privacy invariants.", "ContractVerificationReport"),
			"/api/openapi.json":              getOperation("contracts", "Get OpenAPI document", "OpenAPI 3.1 control-plane contract document.", "OpenAPI"),
			"/api/integrations":              getOperation("contracts", "Get integration catalog", "Privacy-safe integration capability catalog.", "CapabilityCatalog"),
			"/api/runtime/status":            getOperation("contracts", "Get runtime status", "Process-local observer/control-plane mode and compatibility hashes.", "RuntimeStatus"),
			"/api/event-schema":              getOperation("canonical-events", "Get canonical event schema", "Metadata-only canonical event contract and supported event types.", "CanonicalEventSchema"),
			"/api/event-examples":            eventExamplesOperation(),
			"/api/events/validate":           canonicalEventPostOperation("canonical-events", "Validate canonical events", "Validate one or more canonical events without writing SQLite.", false),
			"/api/events":                    canonicalEventPostOperation("canonical-events", "Ingest canonical events", "Ingest one or more metadata-only canonical events.", true),
			"/api/integrations/adapter-spec": getOperation("adapter-conformance", "Get adapter contract", "Machine-readable adapter contract for privacy-safe integrations.", "AdapterContract"),
			"/api/integrations/conformance":  adapterConformanceOperation(),
			"/api/workload-events":           workloadEventsOperation(false),
			"/api/workload-events/stream":    workloadEventsOperation(true),
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"Hash": map[string]interface{}{
					"type":        "string",
					"pattern":     "^sha256:[a-f0-9]{64}$",
					"description": "Stable SHA-256 fingerprint.",
				},
				"DiscoveryManifest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
					"required":             []string{"contract", "version", "local_first", "contract_bundle_uri", "capability_catalog_hash", "canonical_schema_hash", "adapter_spec_hash"},
					"properties": map[string]interface{}{
						"contract":                constSchema("agent-ledger.discovery"),
						"version":                 stringSchema(),
						"contract_bundle_uri":     stringSchema(),
						"capability_catalog_hash": refSchema("Hash"),
						"canonical_schema_hash":   refSchema("Hash"),
						"adapter_spec_hash":       refSchema("Hash"),
						"prompt_content_stored":   boolSchema(),
						"usage_data_uploaded":     boolSchema(),
					},
				},
				"ContractBundle": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
					"required":             []string{"contract", "version", "bundle_hash", "documents"},
					"properties": map[string]interface{}{
						"contract":    constSchema("agent-ledger.contract-bundle"),
						"version":     stringSchema(),
						"bundle_hash": refSchema("Hash"),
						"documents": map[string]interface{}{
							"type":  "array",
							"items": refSchema("ContractDocument"),
						},
					},
				},
				"ContractDocument": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
					"required":             []string{"id", "contract", "version", "hash", "primary_uri", "read_only_safe", "writes_local_state"},
					"properties": map[string]interface{}{
						"id":                 stringSchema(),
						"name":               stringSchema(),
						"contract":           stringSchema(),
						"version":            stringSchema(),
						"hash":               refSchema("Hash"),
						"primary_uri":        stringSchema(),
						"read_only_safe":     boolSchema(),
						"writes_local_state": boolSchema(),
					},
				},
				"ContractVerificationReport": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
					"required":             []string{"contract", "version", "ok", "checked", "failed", "bundle_hash", "openapi_hash", "checks"},
					"properties": map[string]interface{}{
						"contract":     constSchema("agent-ledger.contract-verification"),
						"version":      stringSchema(),
						"ok":           boolSchema(),
						"checked":      map[string]interface{}{"type": "integer", "minimum": 0},
						"failed":       map[string]interface{}{"type": "integer", "minimum": 0},
						"bundle_hash":  refSchema("Hash"),
						"openapi_hash": refSchema("Hash"),
						"read_only":    boolSchema(),
						"privacy":      stringSchema(),
						"checks": map[string]interface{}{
							"type":  "array",
							"items": refSchema("ContractVerificationCheck"),
						},
					},
				},
				"ContractVerificationCheck": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"name", "ok", "severity", "message"},
					"properties": map[string]interface{}{
						"name":     stringSchema(),
						"ok":       boolSchema(),
						"severity": stringSchema(),
						"message":  stringSchema(),
						"expected": stringSchema(),
						"actual":   stringSchema(),
					},
				},
				"CapabilityCatalog":    looseObjectSchema("Integration capability catalog."),
				"RuntimeStatus":        looseObjectSchema("Process-local runtime mode and compatibility hashes."),
				"CanonicalEventSchema": looseObjectSchema("Canonical event contract metadata."),
				"AdapterContract":      looseObjectSchema("Machine-readable adapter contract."),
				"CanonicalEvent": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"source", "event_type", "payload"},
					"properties": map[string]interface{}{
						"source":          stringSchema(),
						"event_type":      stringSchema(),
						"event_id":        stringSchema(),
						"schema_version":  stringSchema(),
						"source_version":  stringSchema(),
						"parser_version":  stringSchema(),
						"source_event_id": stringSchema(),
						"raw_ref":         stringSchema(),
						"match_type":      stringSchema(),
						"workload_id":     stringSchema(),
						"agent_run_id":    stringSchema(),
						"session_id":      stringSchema(),
						"model":           stringSchema(),
						"project":         stringSchema(),
						"git_branch":      stringSchema(),
						"timestamp":       stringSchema(),
						"confidence":      numberSchema(),
						"payload":         looseObjectSchema("Metadata-only event payload. Raw prompt/content fields are rejected by the server."),
					},
				},
				"CanonicalEventRequest": map[string]interface{}{
					"oneOf": []map[string]interface{}{
						refSchema("CanonicalEvent"),
						{"type": "array", "items": refSchema("CanonicalEvent"), "maxItems": 500},
					},
				},
				"ValidationResponse": looseObjectSchema("Validation result for one or more canonical events."),
				"IngestResponse":     looseObjectSchema("Ingest result for one or more canonical events."),
				"WorkloadEventFeed":  looseObjectSchema("Cursor-stable workload state feed."),
				"Error": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"error": stringSchema()},
				},
				"OpenAPI": looseObjectSchema("OpenAPI 3.1 document."),
			},
		},
	}
}

func OpenAPIFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	return hashJSONPayload(OpenAPISpecFor(opts, runtime))
}

func getOperation(tag, summary, description, schema string) map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{tag},
			"summary":     summary,
			"description": description,
			"responses": map[string]interface{}{
				"200": jsonResponse(schema),
				"304": map[string]interface{}{"description": "Not modified when If-None-Match matches the current ETag."},
			},
		},
	}
}

func eventExamplesOperation() map[string]interface{} {
	op := getOperation("canonical-events", "Get canonical event examples", "Privacy-safe canonical event examples.", "CanonicalEventSchema")
	op["get"].(map[string]interface{})["parameters"] = []map[string]interface{}{
		queryParam("type", "Filter examples by event type."),
		queryParam("event_type", "Alias for type."),
	}
	return op
}

func canonicalEventPostOperation(tag, summary, description string, writes bool) map[string]interface{} {
	op := map[string]interface{}{
		"post": map[string]interface{}{
			"tags":        []string{tag},
			"summary":     summary,
			"description": description,
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": writes,
				"read_only_safe":     !writes,
				"max_events":         500,
				"max_body_bytes":     4 << 20,
			},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{"schema": refSchema("CanonicalEventRequest")},
				},
			},
			"responses": map[string]interface{}{
				"200": jsonResponse(map[bool]string{true: "IngestResponse", false: "ValidationResponse"}[writes]),
				"400": jsonResponse("Error"),
			},
		},
	}
	return op
}

func adapterConformanceOperation() map[string]interface{} {
	return map[string]interface{}{
		"post": map[string]interface{}{
			"tags":        []string{"adapter-conformance"},
			"summary":     "Validate adapter fixture",
			"description": "Validate canonical, provider, provider-stream, OpenTelemetry GenAI, or A2A adapter fixture without writing SQLite.",
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": false,
				"read_only_safe":     true,
				"max_body_bytes":     4 << 20,
			},
			"parameters": []map[string]interface{}{
				queryParam("kind", "auto, canonical, provider, provider-stream, otel, or a2a."),
				boolQueryParam("strict", "Treat provenance warnings as validation failures."),
			},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json":     map[string]interface{}{"schema": looseObjectSchema("Adapter fixture JSON.")},
					"text/event-stream":    map[string]interface{}{"schema": stringSchema()},
					"application/x-ndjson": map[string]interface{}{"schema": stringSchema()},
				},
			},
			"responses": map[string]interface{}{
				"200": jsonResponse("ValidationResponse"),
				"400": jsonResponse("Error"),
			},
		},
	}
}

func workloadEventsOperation(stream bool) map[string]interface{} {
	method := map[string]interface{}{
		"tags":        []string{"workload-feed"},
		"summary":     "Get workload event feed",
		"description": "Cursor-stable metadata-only workload state feed for local monitors and agent routers.",
		"parameters": []map[string]interface{}{
			queryParam("from", "YYYY-MM-DD lower bound."),
			queryParam("to", "YYYY-MM-DD upper bound."),
			queryParam("source", "Optional source filter."),
			queryParam("model", "Optional model filter."),
			queryParam("project", "Optional project filter."),
			queryParam("phase", "Optional workload phase filter."),
			queryParam("severity", "Optional severity filter."),
			queryParam("cursor", "Previously returned feed cursor."),
			queryParam("stale_after", "Duration such as 10m."),
			intQueryParam("limit", "Maximum rows."),
		},
		"responses": map[string]interface{}{
			"200": jsonResponse("WorkloadEventFeed"),
			"304": map[string]interface{}{"description": "Not modified when cursor or If-None-Match matches."},
			"400": jsonResponse("Error"),
		},
	}
	if stream {
		method["summary"] = "Stream workload event feed"
		method["responses"] = map[string]interface{}{
			"200": map[string]interface{}{
				"description": "SSE stream that emits workload_events messages with the feed cursor as SSE id.",
				"content": map[string]interface{}{
					"text/event-stream": map[string]interface{}{"schema": stringSchema()},
				},
			},
			"400": jsonResponse("Error"),
		}
	}
	return map[string]interface{}{"get": method}
}

func jsonResponse(schema interface{}) map[string]interface{} {
	ref := schema
	if name, ok := schema.(string); ok {
		ref = refSchema(name)
	}
	return map[string]interface{}{
		"description": "JSON response.",
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{"schema": ref},
		},
	}
}

func queryParam(name, description string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"in":          "query",
		"description": description,
		"required":    false,
		"schema":      stringSchema(),
	}
}

func boolQueryParam(name, description string) map[string]interface{} {
	param := queryParam(name, description)
	param["schema"] = boolSchema()
	return param
}

func intQueryParam(name, description string) map[string]interface{} {
	param := queryParam(name, description)
	param["schema"] = map[string]interface{}{"type": "integer", "minimum": 1}
	return param
}

func refSchema(name string) map[string]interface{} {
	return map[string]interface{}{"$ref": "#/components/schemas/" + name}
}

func constSchema(value string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "const": value}
}

func stringSchema() map[string]interface{} {
	return map[string]interface{}{"type": "string"}
}

func boolSchema() map[string]interface{} {
	return map[string]interface{}{"type": "boolean"}
}

func numberSchema() map[string]interface{} {
	return map[string]interface{}{"type": "number"}
}

func looseObjectSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          description,
		"additionalProperties": true,
	}
}
