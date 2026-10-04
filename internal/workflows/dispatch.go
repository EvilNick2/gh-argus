package workflows

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Input is one workflow_dispatch input as declared in the workflow file.
type Input struct {
	Name        string
	Description string
	Type        string // string, boolean, choice, number or environment
	Default     string
	Required    bool
	Options     []string
}

// DispatchSpec is what a workflow file says about manual dispatch.
type DispatchSpec struct {
	Dispatchable bool
	Inputs       []Input
}

// NeedsInput reports a required input with no default, which the user must
// fill in before dispatching.
func (d DispatchSpec) NeedsInput() bool {
	for _, in := range d.Inputs {
		if in.Required && in.Default == "" {
			return true
		}
	}
	return false
}

// ParseDispatch reads the workflow_dispatch trigger of a workflow file. It
// walks the node tree so inputs keep the order the file declares them in,
// which decoding into a map would lose.
func ParseDispatch(src []byte) (DispatchSpec, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return DispatchSpec{}, err
	}
	var spec DispatchSpec
	if len(doc.Content) == 0 {
		return spec, nil
	}
	on := mapValue(doc.Content[0], "on")
	if on == nil {
		return spec, nil
	}
	switch on.Kind {
	case yaml.ScalarNode:
		spec.Dispatchable = on.Value == "workflow_dispatch"
	case yaml.SequenceNode:
		for _, n := range on.Content {
			if n.Value == "workflow_dispatch" {
				spec.Dispatchable = true
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(on.Content); i += 2 {
			if on.Content[i].Value == "workflow_dispatch" {
				spec.Dispatchable = true
				spec.Inputs = parseInputs(mapValue(on.Content[i+1], "inputs"))
			}
		}
	}
	return spec, nil
}

func parseInputs(n *yaml.Node) []Input {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	var out []Input
	for i := 0; i+1 < len(n.Content); i += 2 {
		in := Input{Name: n.Content[i].Value, Type: "string"}
		def := n.Content[i+1]
		if v := mapValue(def, "description"); v != nil {
			in.Description = v.Value
		}
		if v := mapValue(def, "type"); v != nil {
			in.Type = v.Value
		}
		if v := mapValue(def, "default"); v != nil {
			in.Default = v.Value
		}
		if v := mapValue(def, "required"); v != nil {
			in.Required = v.Value == "true"
		}
		if v := mapValue(def, "options"); v != nil {
			for _, o := range v.Content {
				in.Options = append(in.Options, o.Value)
			}
		}
		out = append(out, in)
	}
	return out
}

// mapValue returns the value node for key in mapping n, or nil.
func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// DecodeContent extracts a file from a GET /repos/{owner}/{repo}/contents
// response. The default media type only carries content for files up to
// 1 MB, far above any workflow file.
func DecodeContent(body []byte) ([]byte, error) {
	var file struct {
		Encoding string
		Content  string
	}
	if err := json.Unmarshal(body, &file); err != nil {
		return nil, err
	}
	if file.Encoding != "base64" {
		return nil, fmt.Errorf("workflow file has encoding %q, want base64", file.Encoding)
	}
	return base64.StdEncoding.DecodeString(file.Content)
}
