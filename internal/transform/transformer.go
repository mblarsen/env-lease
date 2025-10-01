package transform

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
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
type selectTransformer struct{ Path string }
type explodeTransformer struct {
	args explodeArgs
}

type structuredData map[string]interface{}
type ExplodedData map[string]string

var blacklistedVars = map[string]bool{
	"PATH":  true,
	"HOME":  true,
	"SHELL": true,
	"USER":  true,
	"TERM":  true,
	"PWD":   true,
}

var blacklistedPrefixes = []string{
	"XDG_",
	"LC_",
}

type explodeArgs struct {
	FilterPrefix string
	AddPrefix    string
}

func parseExplodeArgs(argStr string) (explodeArgs, error) {
	args := explodeArgs{}
	argStr = strings.TrimSpace(argStr)
	if argStr == "" {
		return args, nil
	}

	parts := strings.Split(argStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return args, fmt.Errorf("invalid argument format in explode: %s", part)
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		if value == "" {
			return args, fmt.Errorf("empty value for argument in explode: %s", key)
		}

		switch key {
		case "filter":
			args.FilterPrefix = value
		case "prefix":
			args.AddPrefix = value
		default:
			return args, fmt.Errorf("unknown argument in explode: %s", key)
		}
	}
	return args, nil
}

func NewPipeline(transformations []string) (*Pipeline, error) {
	p := &Pipeline{}
	hasExplode := false
	for i, name := range transformations {
		if hasExplode {
			return nil, fmt.Errorf("'explode' must be the final transform in a pipeline")
		}
		t, err := newTransformer(name)
		if err != nil {
			return nil, err
		}
		if _, ok := t.(*explodeTransformer); ok {
			hasExplode = true
			// Explode must be last, except we allow a final to_... transform
			if i < len(transformations)-1 {
				return nil, fmt.Errorf("'explode' must be the final transform in a pipeline")
			}
		}
		p.transformers = append(p.transformers, t)
	}
	return p, nil
}

func (p *Pipeline) Run(input string) (interface{}, error) {
	var current interface{} = input
	var err error
	for _, t := range p.transformers {
		current, err = t.Transform(current)
		if err != nil {
			return nil, err
		}
	}
	return current, nil
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
	case name == "explode" || strings.HasPrefix(name, "explode("):
		var args explodeArgs
		var err error
		if strings.HasPrefix(name, "explode(") {
			if !strings.HasSuffix(name, ")") {
				return nil, fmt.Errorf("invalid explode syntax: missing closing parenthesis")
			}
			argStr := strings.TrimPrefix(name, "explode(")
			argStr = strings.TrimSuffix(argStr, ")")
			args, err = parseExplodeArgs(argStr)
			if err != nil {
				return nil, err
			}
		}
		return &explodeTransformer{args: args}, nil
	default:
		return nil, fmt.Errorf("unknown transformer: %s", name)
	}
}

func (t *explodeTransformer) Transform(input interface{}) (interface{}, error) {
	data, ok := input.(structuredData)
	if !ok {
		return nil, fmt.Errorf("explode: input must be structured data (e.g., from a 'json' or 'toml' transform)")
	}

	result := make(ExplodedData)
	for key, value := range data {
		// 1. Filter
		if t.args.FilterPrefix != "" && !strings.HasPrefix(strings.ToUpper(key), strings.ToUpper(t.args.FilterPrefix)) {
			continue
		}

		// 2. Prefix
		finalKey := t.args.AddPrefix + strings.ToUpper(key)

		// 3. Blacklist validation
		if blacklistedVars[finalKey] {
			return nil, fmt.Errorf("explode: generated variable '%s' is a protected system variable", finalKey)
		}
		for _, prefix := range blacklistedPrefixes {
			if strings.HasPrefix(finalKey, prefix) {
				return nil, fmt.Errorf("explode: generated variable '%s' uses a protected system prefix '%s'", finalKey, prefix)
			}
		}

		// Ensure key is a valid environment variable segment
		if !isValidEnvVar(key) {
			return nil, fmt.Errorf("explode: key '%s' is not a valid environment variable name", key)
		}

		// Ensure value is not complex
		if _, isMap := value.(map[string]interface{}); isMap {
			return nil, fmt.Errorf("explode: nested objects are not supported (key: '%s')", key)
		}
		if _, isSlice := value.([]interface{}); isSlice {
			return nil, fmt.Errorf("explode: arrays are not supported (key: '%s')", key)
		}

		result[finalKey] = fmt.Sprintf("%v", value)
	}
	return result, nil
}

func isValidEnvVar(s string) bool {
	return !strings.ContainsAny(s, " =")
}

func (t *toJsonTransformer) Transform(input interface{}) (interface{}, error) {
	var data interface{}
	switch v := input.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &data); err != nil {
			return nil, fmt.Errorf("to_json: failed to parse input json: %w", err)
		}
	case structuredData:
		data = v
	default:
		return nil, fmt.Errorf("to_json: input must be a string or structured data")
	}

	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("to_json: %w", err)
	}
	return string(jsonBytes), nil
}

func (t *toYamlTransformer) Transform(input interface{}) (interface{}, error) {
	var data interface{}
	switch v := input.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &data); err != nil {
			return nil, fmt.Errorf("to_yaml: failed to parse input json: %w", err)
		}
	case structuredData:
		data = v
	default:
		return nil, fmt.Errorf("to_yaml: input must be a string or structured data")
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("to_yaml: %w", err)
	}
	return string(yamlBytes), nil
}

func (t *toTomlTransformer) Transform(input interface{}) (interface{}, error) {
	var data interface{}
	switch v := input.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &data); err != nil {
			return nil, fmt.Errorf("to_toml: failed to parse input json: %w", err)
		}
	case structuredData:
		data = v
	default:
		return nil, fmt.Errorf("to_toml: input must be a string or structured data")
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
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("select: failed to convert back to json: %w", err)
	}
	result := gjson.Get(string(jsonBytes), t.Path)
	if !result.Exists() {
		return nil, fmt.Errorf("select: path not found: %s", t.Path)
	}

	if result.IsObject() {
		var data structuredData
		if err := json.Unmarshal([]byte(result.Raw), &data); err != nil {
			return nil, fmt.Errorf("select: failed to re-unmarshal selected object: %w", err)
		}
		return data, nil
	}

	if result.IsArray() {
		return result.Raw, nil
	}

	return result.String(), nil
}
