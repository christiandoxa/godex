package main

import "strings"

func deepSeekSchemaAliasSuffixes04390() []string {
	return []string{
		"schema-alias-json-flat", "schema-alias-input-flat", "schema-alias-schema-flat",
		"schema-alias-json-nested", "schema-alias-input-nested", "schema-alias-schema-nested",
		"schema-alias-json-normal", "schema-alias-input-normal", "schema-alias-schema-normal",
		"schema-alias-priority", "schema-alias-null-primary", "schema-alias-null-shadow",
		"schema-alias-absent", "schema-alias-strict-false",
	}
}

func deepSeekSchemaAliasFixture04390(suffix string) (requestPrimitiveFixture04384, bool) {
	const schema = `{"properties":{"id":{"enum":[9007199254740993]}},"type":"object"}`
	alias, nested, normal := "parametersJsonSchema", false, false
	fields, wire := `"parametersJsonSchema":`+schema, `{"name":"lookup","parameters":`+schema+`}`
	reject := false
	switch suffix {
	case "schema-alias-priority":
		fields = `"parameters":` + schema + `,"parametersJsonSchema":null,"input_schema":null,"schema":null`
	case "schema-alias-null-primary":
		fields, reject = `"parameters":null,"parametersJsonSchema":`+schema, true
	case "schema-alias-null-shadow":
		fields, reject = `"parametersJsonSchema":null,"input_schema":`+schema, true
	case "schema-alias-absent":
		nested, fields, wire = true, "", `{"name":"lookup"}`
	case "schema-alias-strict-false":
		nested = true
		fields += `,"strict":false`
		wire = `{"name":"lookup","parametersJsonSchema":` + schema + `,"strict":false}`
	default:
		parts := strings.Split(strings.TrimPrefix(suffix, "schema-alias-"), "-")
		if len(parts) != 2 {
			return requestPrimitiveFixture04384{}, false
		}
		switch parts[0] {
		case "json":
		case "input":
			alias = "input_schema"
		case "schema":
			alias = "schema"
		default:
			return requestPrimitiveFixture04384{}, false
		}
		switch parts[1] {
		case "flat":
		case "nested", "normal":
			nested, normal = true, parts[1] == "normal"
		default:
			return requestPrimitiveFixture04384{}, false
		}
		fields = `"` + alias + `":` + schema
		if nested && !normal {
			wire = `{"name":"lookup",` + fields + `}`
			if alias == "input_schema" {
				wire = `{` + fields + `,"name":"lookup"}`
			}
		}
	}
	function := `"name":"lookup"`
	if fields != "" {
		function += `,` + fields
	}
	tool := `{"type":"function",` + function + `}`
	if nested {
		tool = `{"type":"function","function":{` + function + `}}`
	}
	inputFields, wireFields := "", ""
	if normal {
		inputFields, wireFields = `,"safety_identifier":"synthetic_user"`, `,"user_id":"synthetic_user"`
	}
	return requestPrimitiveFixture04384{
		body:     `{"model":"gemini-3.1-pro-preview","input":"same request","stream":false,"tools":[` + tool + `]` + inputFields + `}`,
		wireBody: `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false,"tools":[{"function":` + wire + `,"type":"function"}]` + wireFields + `}`,
		reject:   reject,
	}, true
}
