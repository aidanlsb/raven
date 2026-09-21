package schemamigratesvc

import "gopkg.in/yaml.v3"

func decodeYAMLMap(data []byte) (map[string]interface{}, bool) {
	var values map[string]interface{}
	if yaml.Unmarshal(data, &values) != nil {
		return nil, false
	}
	if values == nil {
		values = make(map[string]interface{})
	}
	return values, true
}

func marshalYAMLMap(values map[string]interface{}) ([]byte, bool) {
	data, err := yaml.Marshal(values)
	return data, err == nil
}
