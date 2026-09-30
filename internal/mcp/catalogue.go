package mcp

import "sort"

// Argument is one input of a tool, described for a person rather than for a
// client parser.
type Argument struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Choices     []string
}

// Tool is one capability, in the shape a page can render.
type Tool struct {
	Name        string
	Description string
	Arguments   []Argument
}

// Catalogue describes every tool the server exposes.
//
// It is derived from the same definitions the protocol serves rather than
// written alongside them, because a hand-kept list of capabilities is a list
// that quietly stops being true: a tool gains an argument, the page still shows
// the old one, and the documentation becomes a thing people learn not to trust.
// Reading the definitions means the page is wrong only if the server is.
func Catalogue() []Tool {
	definitions := toolDefinitions()
	tools := make([]Tool, 0, len(definitions))
	for _, definition := range definitions {
		entry, ok := definition.(map[string]any)
		if !ok {
			continue
		}
		tool := Tool{Name: text(entry["name"]), Description: text(entry["description"])}
		tool.Arguments = readArguments(entry["inputSchema"])
		tools = append(tools, tool)
	}
	return tools
}

// readArguments flattens one JSON Schema into the arguments of a tool. Required
// arguments are listed first, then alphabetically, so the shape of a call is
// readable at a glance instead of in the order the schema happened to be
// written.
func readArguments(raw any) []Argument {
	schema, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		return nil
	}
	required := map[string]bool{}
	if names, ok := schema["required"].([]string); ok {
		for _, name := range names {
			required[name] = true
		}
	}
	arguments := make([]Argument, 0, len(properties))
	for name, value := range properties {
		field, _ := value.(map[string]any)
		argument := Argument{Name: name, Type: text(field["type"]), Description: text(field["description"]), Required: required[name]}
		if choices, ok := field["enum"].([]string); ok {
			argument.Choices = choices
		}
		if argument.Type == "array" {
			if items, ok := field["items"].(map[string]any); ok && text(items["type"]) != "" {
				argument.Type = text(items["type"]) + "[]"
			}
		}
		arguments = append(arguments, argument)
	}
	sort.Slice(arguments, func(i, j int) bool {
		if arguments[i].Required != arguments[j].Required {
			return arguments[i].Required
		}
		return arguments[i].Name < arguments[j].Name
	})
	return arguments
}

func text(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
