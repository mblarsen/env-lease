package transform

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
	"strings"
)

// Transformer is an interface for transforming data.
type Transformer interface {
	Transform(input interface{}) (interface{}, error)
}

// Pipeline holds a series of transformers.
type Pipeline struct {
	transformers []Transformer
}

type base64EncodeTransformer struct{}
type base64DecodeTransformer struct{}
type jsonTransformer struct{}
type tomlTransformer struct{}
type yamlTransformer struct{}
type toJsonTransformer struct{}
type toYamlTransformer struct{}
type toTomlTransformer struct{}
type selectTransformer struct {
	Path string
}
type structuredData map[string]interface{}


// NewPipeline creates a new transformation pipeline.
func NewPipeline(transformations []string) (*Pipeline, error) {
	var transformers []Transformer
	for _, t := range transformations {
		transformer, err := newTransformer(t)
		if err != nil {
			return nil, err
		}
		transformers = append(transformers, transformer)
	}
	return &Pipeline{transformers: transformers}, nil
}

// Run executes the pipeline on the given input.
func (p *Pipeline) Run(input string) (string, error) {
	var current interface{} = input
	for _, t := range p.transformers {
		var err error
		current, err = t.Transform(current)
		if err != nil {
			return "", err
		}
	}

	output, ok := current.(string)
	if !ok {
		return "", fmt.Errorf("pipeline did not produce a string output")
	}
	return output, nil
}

func newTransformer(name string) (Transformer, error) {
	switch {
	case name == "base64-encode":
		return &base64EncodeTransformer{}, nil
	case name == "base64-decode":
		return &base64DecodeTransformer{}, nil
	case name == "json":
		return &jsonTransformer{}, nil
	case name == "toml":
		return &tomlTransformer{}, nil
	case name == "yaml":
		return &yamlTransformer{}, nil
	case name == "to_json":
		return &toJsonTransformer{}, nil
	case name == "to_yaml":
		return &toYamlTransformer{}, nil
	case name == "to_toml":
		return &toTomlTransformer{}, nil
	case strings.HasPrefix(name, "select "):
		path := strings.TrimPrefix(name, "select ")
		return &selectTransformer{Path: strings.Trim(path, "'\"")}, nil
	default:
		return nil, fmt.Errorf("unknown transformer: %s", name)
	}
}

func (t *toJsonTransformer) Transform(input interface{}) (interface{}, error) {
	s, ok := input.(string)
	if !ok {
		return nil, fmt.Errorf("to_json: input must be a string")
	}
	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return nil, fmt.Errorf("to_json: failed to parse input json: %w", err)
	}
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("to_json: %w", err)
	}
	return string(jsonBytes), nil
}

func (t *toYamlTransformer) Transform(input interface{}) (interface{}, error) {
	s, ok := input.(string)
	if !ok {
		return nil, fmt.Errorf("to_yaml: input must be a string")
	}
	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return nil, fmt.Errorf("to_yaml: failed to parse input json: %w", err)
	}
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("to_yaml: %w", err)
	}
	return string(yamlBytes), nil
}

func (t *toTomlTransformer) Transform(input interface{}) (interface{}, error) {
	s, ok := input.(string)
	if !ok {
		return nil, fmt.Errorf("to_toml: input must be a string")
	}
	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return nil, fmt.Errorf("to_toml: failed to parse input json: %w", err)
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(data); err != nil {
		return nil, fmt.Errorf("to_toml: %w", err)
	}
	return buf.String(), nil
}


func (t *base64EncodeTransformer) Transform(input interface{}) (interface{}, error) {
	s, ok := input.(string)
	if !ok {
		return nil, fmt.Errorf("base64-encode: input must be a string")
	}
	return base64.StdEncoding.EncodeToString([]byte(s)), nil
}

func (t *base64DecodeTransformer) Transform(input interface{}) (interface{}, error) {
	s, ok := input.(string)
	if !ok {
		return nil, fmt.Errorf("base64-decode: input must be a string")
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64-decode: %w", err)
	}
	return string(decoded), nil
}

func (t *jsonTransformer) Transform(input interface{}) (interface{}, error) {
	s, ok := input.(string)
	if !ok {
		return nil, fmt.Errorf("json: input must be a string")
	}
	var data structuredData
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	return data, nil
}

func (t *tomlTransformer) Transform(input interface{}) (interface{}, error) {
	s, ok := input.(string)
	if !ok {
		return nil, fmt.Errorf("toml: input must be a string")
	}
	var data structuredData
	if err := toml.Unmarshal([]byte(s), &data); err != nil {
		return nil, fmt.Errorf("toml: %w", err)
	}
	return data, nil
}

func (t *yamlTransformer) Transform(input interface{}) (interface{}, error) {
	s, ok := input.(string)
	if !ok {
		return nil, fmt.Errorf("yaml: input must be a string")
	}
	var data structuredData
	if err := yaml.Unmarshal([]byte(s), &data); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	return data, nil
}

func (t *selectTransformer) Transform(input interface{}) (interface{}, error) {
	data, ok := input.(structuredData)
	if !ok {
		return nil, fmt.Errorf("select: input must be structured data")
	}
	// Convert back to JSON to use gjson
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("select: failed to convert back to json: %w", err)
	}
	result := gjson.Get(string(jsonBytes), t.Path)
	if !result.Exists() {
		return nil, fmt.Errorf("select: path not found: %s", t.Path)
	}

	// If the result is a complex object, return its raw JSON string representation.
	// Otherwise, return its simple string representation.
	if result.IsObject() || result.IsArray() {
		return result.Raw, nil
	}

	return result.String(), nil
}
