package deepseek

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func deepSeekTools(object map[string]any, strictTools bool) ([]any, map[string]bool, error) {
	value, ok := object["tools"]
	if !ok {
		return nil, map[string]bool{}, nil
	}
	items, valid := value.([]any)
	if !valid {
		return nil, nil, errors.New("DeepSeek tools must be an array")
	}
	result := make([]any, 0, len(items))
	names := make(map[string]bool)
	byName := make(map[string]map[string]any)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, errors.New("DeepSeek tools entries must be objects")
		}
		converted, err := deepSeekFunctionTool(item, strictTools)
		if err != nil {
			return nil, nil, err
		}
		name := converted[deepSeekFunctionKey].(map[string]any)["name"].(string)
		if previous, exists := byName[name]; exists {
			if mapsEquivalent(previous, converted) {
				continue
			}
			return nil, nil, fmt.Errorf("DeepSeek function tool name `%s` is duplicated after translation", name)
		}
		byName[name] = converted
		names[name] = true
		result = append(result, converted)
	}
	return result, names, nil
}

func deepSeekFunctionTool(item map[string]any, strictTools bool) (map[string]any, error) {
	kind, function, nested, err := deepSeekFunctionObject(item)
	if err != nil {
		return nil, err
	}
	_ = kind
	name, err := deepSeekFunctionName(function)
	if err != nil {
		return nil, err
	}
	resultFunction, err := deepSeekFunctionMetadata(function, name)
	if err != nil {
		return nil, err
	}
	parameters, err := deepSeekFunctionParameters(function, name)
	if err != nil {
		return nil, err
	}
	if err := applyDeepSeekStrictTool(item, function, nested, name, strictTools, parameters, resultFunction); err != nil {
		return nil, err
	}
	resultFunction["parameters"] = parameters
	return map[string]any{"type": deepSeekFunctionKey, deepSeekFunctionKey: resultFunction}, nil
}

func deepSeekFunctionObject(item map[string]any) (string, map[string]any, bool, error) {
	kind, _ := item["type"].(string)
	if kind == "" {
		kind = deepSeekFunctionKey
	}
	if kind != deepSeekFunctionKey {
		return "", nil, false, fmt.Errorf("DeepSeek tool type `%s` is not supported", kind)
	}
	function, nested := item[deepSeekFunctionKey].(map[string]any)
	if !nested {
		function = item
	}
	return kind, function, nested, nil
}

func deepSeekFunctionName(function map[string]any) (string, error) {
	name, ok := function["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return "", errors.New("DeepSeek function tools require a name")
	}
	if err := validateFunctionName(name); err != nil {
		return "", err
	}
	return name, nil
}

func deepSeekFunctionMetadata(function map[string]any, name string) (map[string]any, error) {
	result := map[string]any{"name": name}
	description, found := function[deepSeekDescriptionKey]
	if !found {
		return result, nil
	}
	text, valid := description.(string)
	if !valid {
		return nil, errors.New("DeepSeek function description must be a string")
	}
	result[deepSeekDescriptionKey] = text
	return result, nil
}

func deepSeekFunctionParameters(function map[string]any, name string) (map[string]any, error) {
	parameters := map[string]any{"type": "object", deepSeekPropertiesKey: map[string]any{}, deepSeekAdditionalPropertiesKey: true}
	raw, found := function["parameters"]
	if !found {
		return parameters, nil
	}
	object, valid := raw.(map[string]any)
	if !valid {
		return nil, fmt.Errorf("DeepSeek function tool `%s` parameters must be an object", name)
	}
	return cloneMap(object), nil
}

func applyDeepSeekStrictTool(
	item, function map[string]any,
	nested bool,
	name string,
	strictTools bool,
	parameters, resultFunction map[string]any,
) error {
	strict, err := strictToolFlag(item, function, nested)
	if err != nil {
		return err
	}
	if strict && !strictTools {
		return fmt.Errorf("DeepSeek strict function tool `%s` requires deepseek.strict_tools=true", name)
	}
	if !strictTools {
		return nil
	}
	if err := normalizeStrictSchema(parameters, name); err != nil {
		return err
	}
	resultFunction[deepSeekStrictKey] = true
	return nil
}

func strictToolFlag(item, function map[string]any, nested bool) (bool, error) {
	strict := false
	if raw, found := function[deepSeekStrictKey]; found {
		flag, valid := raw.(bool)
		if !valid {
			return false, errors.New("DeepSeek function strict must be a boolean")
		}
		strict = flag
	}
	if raw, found := item[deepSeekStrictKey]; found && !nested {
		flag, valid := raw.(bool)
		if !valid {
			return false, errors.New("DeepSeek tool strict must be a boolean")
		}
		strict = flag
	}
	return strict, nil
}

func deepSeekToolChoice(object map[string]any, names map[string]bool, thinking bool) (any, error) {
	value, ok := object["tool_choice"]
	if !ok || value == nil || thinking {
		return nil, nil
	}
	switch typed := value.(type) {
	case string:
		return deepSeekStringToolChoice(typed)
	case map[string]any:
		return deepSeekNamedToolChoice(typed, names)
	default:
		return nil, errors.New("DeepSeek tool_choice must be a string or object")
	}
}

func deepSeekStringToolChoice(value string) (any, error) {
	switch value {
	case "auto", "none", deepSeekRequiredKey:
		return value, nil
	default:
		return nil, fmt.Errorf("DeepSeek tool_choice string `%s` is not supported", value)
	}
}

func deepSeekNamedToolChoice(value map[string]any, names map[string]bool) (any, error) {
	kind, _ := value["type"].(string)
	if kind == "" {
		kind = deepSeekFunctionKey
	}
	if kind != deepSeekFunctionKey {
		return nil, fmt.Errorf("DeepSeek tool_choice type `%s` is not supported", kind)
	}
	name := deepSeekToolChoiceName(value)
	if name == "" {
		return nil, errors.New("DeepSeek named tool_choice requires a function name")
	}
	if err := validateFunctionName(name); err != nil {
		return nil, err
	}
	if !names[name] {
		return nil, fmt.Errorf("DeepSeek named tool_choice `%s` does not match any translated function tool", name)
	}
	return map[string]any{"type": deepSeekFunctionKey, deepSeekFunctionKey: map[string]any{"name": name}}, nil
}

func deepSeekToolChoiceName(value map[string]any) string {
	if name := firstStringValue(value, "name"); name != "" {
		return name
	}
	function, _ := value[deepSeekFunctionKey].(map[string]any)
	return firstStringValue(function, "name")
}

func validateFunctionName(name string) error {
	if name == "" || len(name) > 64 {
		return invalidFunctionName()
	}
	for _, current := range name {
		if !(current == '_' || current == '-' ||
			current >= 'a' && current <= 'z' ||
			current >= 'A' && current <= 'Z' ||
			current >= '0' && current <= '9') {
			return invalidFunctionName()
		}
	}
	return nil
}

func invalidFunctionName() error {
	return errors.New("DeepSeek function tool names must use only letters, numbers, underscores, or dashes and be at most 64 bytes")
}

func normalizeStrictSchema(schema map[string]any, path string) error {
	if err := rejectStrictKeywords(schema, path); err != nil {
		return err
	}
	if anyOf, found := schema[deepSeekAnyOfKey]; found {
		return normalizeStrictAnyOf(schema, path, anyOf)
	}
	if enumValue, found := schema["enum"]; found {
		if _, valid := enumValue.([]any); !valid {
			return fmt.Errorf("DeepSeek strict tool schema `%s.enum` must be an array", path)
		}
	}
	kind, err := strictSchemaType(schema, path)
	if err != nil {
		return err
	}
	switch kind {
	case "object":
		return normalizeStrictObject(schema, path)
	case "array":
		return normalizeStrictArray(schema, path)
	default:
		return nil
	}
}

func rejectStrictKeywords(schema map[string]any, path string) error {
	allowed := map[string]bool{
		"type": true, deepSeekDescriptionKey: true, deepSeekPropertiesKey: true,
		deepSeekRequiredKey: true, deepSeekAdditionalPropertiesKey: true, deepSeekItemsKey: true,
		"enum": true, deepSeekAnyOfKey: true,
	}
	for key := range schema {
		if !allowed[key] {
			return fmt.Errorf("DeepSeek strict tool schema `%s` uses unsupported keyword `%s`", path, key)
		}
	}
	return nil
}

func normalizeStrictAnyOf(schema map[string]any, path string, raw any) error {
	items, valid := raw.([]any)
	if !valid {
		return fmt.Errorf("DeepSeek strict tool schema `%s.anyOf` must be an array", path)
	}
	for index, value := range items {
		child, valid := value.(map[string]any)
		if !valid {
			return fmt.Errorf("DeepSeek strict tool schema `%s.anyOf[%d]` must be a JSON object", path, index)
		}
		if err := normalizeStrictSchema(child, fmt.Sprintf("%s.anyOf[%d]", path, index)); err != nil {
			return err
		}
		items[index] = child
	}
	schema[deepSeekAnyOfKey] = items
	return nil
}

func strictSchemaType(schema map[string]any, path string) (string, error) {
	kind := "object"
	if raw, found := schema["type"]; found {
		text, valid := raw.(string)
		if !valid {
			return "", fmt.Errorf("DeepSeek strict tool schema `%s.type` must be a string", path)
		}
		kind = text
	}
	switch kind {
	case "object", "string", "number", "integer", "boolean", "array":
		return kind, nil
	default:
		return "", fmt.Errorf("DeepSeek strict tool schema `%s` uses unsupported type `%s`", path, kind)
	}
}

func normalizeStrictObject(schema map[string]any, path string) error {
	properties := map[string]any{}
	if raw, found := schema[deepSeekPropertiesKey]; found {
		object, valid := raw.(map[string]any)
		if !valid {
			return fmt.Errorf("DeepSeek strict tool schema `%s.properties` must be an object", path)
		}
		properties = object
	}
	keys := make([]string, 0, len(properties))
	for name, raw := range properties {
		child, valid := raw.(map[string]any)
		if !valid {
			return fmt.Errorf("DeepSeek strict tool schema `%s.%s` must be a JSON object", path, name)
		}
		if err := normalizeStrictSchema(child, path+"."+name); err != nil {
			return err
		}
		properties[name] = child
		keys = append(keys, name)
	}
	sort.Strings(keys)
	required := make([]any, 0, len(keys))
	for _, key := range keys {
		required = append(required, key)
	}
	schema[deepSeekPropertiesKey] = properties
	schema[deepSeekRequiredKey] = required
	schema[deepSeekAdditionalPropertiesKey] = false
	return nil
}

func normalizeStrictArray(schema map[string]any, path string) error {
	raw, found := schema[deepSeekItemsKey]
	if !found {
		return fmt.Errorf("DeepSeek strict tool schema `%s` array requires items", path)
	}
	items, valid := raw.(map[string]any)
	if !valid {
		return fmt.Errorf("DeepSeek strict tool schema `%s.items` must be a JSON object", path)
	}
	if err := normalizeStrictSchema(items, path+".items"); err != nil {
		return err
	}
	schema[deepSeekItemsKey] = items
	return nil
}

func cloneMap(source map[string]any) map[string]any {
	content, _ := json.Marshal(source)
	var result map[string]any
	_ = json.Unmarshal(content, &result)
	return result
}

func mapsEquivalent(left, right map[string]any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}
