package signature

import "fmt"

// jsonFormatError is returned when JSON does not match expected format.
type jsonFormatError string

func (err jsonFormatError) Error() string {
	return string(err)
}

// validateExactMapKeys returns an error if the keys of m are not exactly expectedKeys, which must be pairwise distinct
func validateExactMapKeys(m map[string]interface{}, expectedKeys ...string) error {
	if len(m) != len(expectedKeys) {
		return jsonFormatError("Unexpected keys in a JSON object")
	}

	for _, k := range expectedKeys {
		if _, ok := m[k]; !ok {
			return jsonFormatError(fmt.Sprintf("Key %s missing in a JSON object", k))
		}
	}
	// Assuming expectedKeys are pairwise distinct, we know m contains len(expectedKeys) different values in expectedKeys.
	return nil
}

// mapField returns a member fieldName of m, if it is a JSON map, or an error.
func mapField(m map[string]interface{}, fieldName string) (map[string]interface{}, error) {
	untyped, ok := m[fieldName]
	if !ok {
		return nil, jsonFormatError(fmt.Sprintf("Field %s missing", fieldName))
	}
	v, ok := untyped.(map[string]interface{})
	if !ok {
		return nil, jsonFormatError(fmt.Sprintf("Field %s is not a JSON object", fieldName))
	}
	return v, nil
}

// stringField returns a member fieldName of m, if it is a string, or an error.
func stringField(m map[string]interface{}, fieldName string) (string, error) {
	untyped, ok := m[fieldName]
	if !ok {
		return "", jsonFormatError(fmt.Sprintf("Field %s missing", fieldName))
	}
	v, ok := untyped.(string)
	if !ok {
		return "", jsonFormatError(fmt.Sprintf("Field %s is not a JSON object", fieldName))
	}
	return v, nil
}
