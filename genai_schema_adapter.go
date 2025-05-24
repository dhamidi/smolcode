package smolcode

import (
	"encoding/json"
	"fmt"
	"strconv"

	"google.golang.org/genai"
)

// intAsStringKeysInSchema lists keys that genai.Schema's UnmarshalJSON expects as strings
// but are standardly numbers in JSON Schema.
var intAsStringKeysInSchema = map[string]bool{
	"minLength":     true,
	"maxLength":     true,
	"minItems":      true,
	"maxItems":      true,
	"minProperties": true,
	"maxProperties": true,
}

// recursiveConvertNumericStringsForSchema traverses a map representing a JSON object.
// It converts numeric values for specific keys to strings for genai.Schema compatibility
// and removes unsupported 'format' values for 'string' types.
func recursiveConvertNumericStringsForSchema(data map[string]interface{}) {
	// Handle 'format' for 'string' type at the current level
	if typeVal, typeOk := data["type"].(string); typeOk && typeVal == "string" {
		if formatVal, formatOk := data["format"].(string); formatOk {
			if formatVal != "enum" && formatVal != "date-time" {
				delete(data, "format")
			}
		}
	}

	for key, value := range data {
		// Existing logic for numeric string conversion
		if intAsStringKeysInSchema[key] {
			if numVal, ok := value.(float64); ok {
				data[key] = strconv.FormatInt(int64(numVal), 10)
			}
		}

		// Recursively process nested objects
		if nestedMap, ok := value.(map[string]interface{}); ok {
			recursiveConvertNumericStringsForSchema(nestedMap)
		}

		// Recursively process objects within arrays
		if nestedArray, ok := value.([]interface{}); ok {
			for _, item := range nestedArray {
				if itemMap, okItemMap := item.(map[string]interface{}); okItemMap {
					recursiveConvertNumericStringsForSchema(itemMap)
				}
			}
		}
	}
}

// DeserializeToolSchema takes a byte slice of JSON, pre-processes it to ensure
// compatibility with genai.Schema's custom UnmarshalJSON method, and then
// unmarshals it into a *genai.Schema object.
func DeserializeToolSchema(jsonBytes []byte) (*genai.Schema, error) {
	if len(jsonBytes) == 0 || string(jsonBytes) == "null" {
		// Return an empty schema, similar to how agent.go handles this.
		return &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}}, nil
	}

	var rawData map[string]interface{}
	// Use json.Unmarshal into rawData first.
	if err := json.Unmarshal(jsonBytes, &rawData); err != nil {
		return nil, fmt.Errorf("DeserializeToolSchema: error unmarshalling to raw map: %w", err)
	}

	recursiveConvertNumericStringsForSchema(rawData)

	modifiedJsonBytes, err := json.Marshal(rawData)
	if err != nil {
		return nil, fmt.Errorf("DeserializeToolSchema: error marshalling modified map: %w", err)
	}

	var schema genai.Schema
	// Now unmarshal the modified JSON using genai.Schema's own UnmarshalJSON.
	// We call schema.UnmarshalJSON directly to ensure its specific logic is used.
	if err := schema.UnmarshalJSON(modifiedJsonBytes); err != nil {
		return nil, fmt.Errorf("DeserializeToolSchema: error unmarshalling to genai.Schema: %w", err)
	}

	return &schema, nil
}
