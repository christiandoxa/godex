package session

import (
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

func TestProdex04358SessionReportMetadataPrecedence(t *testing.T) {
	report := sessionentity.Session{
		ID: "keep-on-turn-context", LastModel: "old-model", LastReasoningEffort: "old-effort",
		UpdatedUnix: 999,
	}
	applySessionMetadata(&report, []byte(`{
		"type":"turn_context",
		"model":" root-model ",
		"effort":" root-effort ",
		"thread_name":" root-name ",
		"cwd":" /root ",
		"model_provider":"root-provider",
		"timestamp":"1970-01-01T01:00:00+01:00",
		"source":{"subagent":{"thread_spawn":{"parent_thread_id":"root-parent"}}},
		"parent_thread_id":"root-direct",
		"payload":{
			"id":"must-not-replace-id",
			"model":" payload-model ",
			"reasoning_effort":" high ",
			"metadata":{"thread_name":" nested-name ","cwd":" /payload-meta ","model_provider":"meta-provider"},
			"model_provider":" payload-provider ",
			"source":{"subagent":{"thread_spawn":{"parent_thread_id":" payload-parent "}}},
			"parent_thread_id":"payload-direct"
		}
	}`))

	if report.ID != "keep-on-turn-context" ||
		report.LastModel != "payload-model" ||
		report.LastReasoningEffort != "high" ||
		report.ThreadName != "nested-name" ||
		report.CWD != "/payload-meta" ||
		report.ModelProvider != "payload-provider" ||
		report.ParentThreadID != "payload-parent" ||
		report.UpdatedAt != "1970-01-01T01:00:00+01:00" ||
		report.UpdatedUnix != 0 {
		t.Fatalf("report = %#v", report)
	}
}

func TestProdex04358SessionReportBlankStringsFallThrough(t *testing.T) {
	report := sessionentity.Session{
		ID: "old-id", ThreadName: "old-name", CWD: "/old", ModelProvider: "old-provider",
		LastModel: "old-model", LastReasoningEffort: "old-effort",
		ParentThreadID: "old-parent", UpdatedAt: "old-time", UpdatedUnix: 777,
	}
	applySessionMetadata(&report, []byte(`{
		"type":"turn_context",
		"model":"root-model",
		"effort":"root-effort",
		"title":"root-title",
		"cwd":"/root",
		"model_provider":"root-provider",
		"updated_at":" ",
		"ts":123,
		"parent_thread_id":"root-direct",
		"payload":{
			"model":" ",
			"effort":"",
			"thread_name":" ",
			"cwd":"",
			"model_provider":" ",
			"source":{"subagent":{"thread_spawn":{"parent_thread_id":""}}},
			"parent_thread_id":"payload-direct"
		}
	}`))

	if report.LastModel != "root-model" || report.LastReasoningEffort != "root-effort" ||
		report.ThreadName != "root-title" || report.CWD != "/root" || report.ModelProvider != "root-provider" ||
		report.ParentThreadID != "payload-direct" || report.UpdatedAt != sessionFormatEpoch(123) || report.UpdatedUnix != 123 {
		t.Fatalf("blank fallback contract = %#v", report)
	}
}

func TestProdex04358SessionReportIDAndNumericTimestampPlanning(t *testing.T) {
	report := sessionentity.Session{ID: "old", UpdatedUnix: -1}
	applySessionMetadata(&report, []byte(`{
		"type":"session_meta",
		"updated_at":123,
		"ts":456,
		"id":"root-id",
		"payload":{"id":" session-one ","session_id":"payload-session"}
	}`))
	if report.ID != "session-one" || report.UpdatedUnix != 123 || report.UpdatedAt != sessionFormatEpoch(123) {
		t.Fatalf("numeric plan = %#v", report)
	}

	applySessionMetadata(&report, []byte(`{"payload":{"id":" session-two "},"timestamp":"456"}`))
	if report.ID != "session-two" || report.UpdatedAt != "456" || report.UpdatedUnix != 456 {
		t.Fatalf("untyped/string timestamp plan = %#v", report)
	}

	applySessionMetadata(&report, []byte(`{"type":"session_meta","payload":{"id":""},"id":"root-id-after-blank"}`))
	if report.ID != "root-id-after-blank" {
		t.Fatalf("blank payload id did not fall through: %#v", report)
	}
}

func TestProdex04358SessionReportGenericObjectsStillApplyGenericMetadata(t *testing.T) {
	report := sessionentity.Session{ID: "session-id", CWD: "/before", ThreadName: "before"}
	applySessionMetadata(&report, []byte(`{
		"type":"response_item",
		"payload":{"cwd":"/after","thread_name":"after","id":"ignored"},
		"model_provider":"provider-after"
	}`))
	if report.ID != "session-id" || report.CWD != "/after" ||
		report.ThreadName != "after" || report.ModelProvider != "provider-after" {
		t.Fatalf("generic metadata = %#v", report)
	}
}

func TestProdex04358SessionReportTypeClassificationDoesNotTrim(t *testing.T) {
	report := sessionentity.Session{ID: "keep", LastModel: "old"}
	applySessionMetadata(&report, []byte(`{
		"type":" turn_context ",
		"payload":{"id":"must-not-replace","model":"must-not-apply","cwd":"/generic"}
	}`))
	if report.ID != "keep" || report.LastModel != "old" || report.CWD != "/generic" {
		t.Fatalf("type classification = %#v", report)
	}
}
