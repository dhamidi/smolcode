package smolcode

import (
	"bytes"
	"fmt"

	"google.golang.org/genai"
)

type ToolDefinition struct {
	Tool     *genai.Tool
	Function func(map[string]any) (map[string]any, error)
}

func (def *ToolDefinition) Name() string {
	return def.Tool.FunctionDeclarations[0].Name
}

type ToolBox map[string]*ToolDefinition

func NewToolBox() ToolBox { return ToolBox{} }

func (tools ToolBox) Add(def *ToolDefinition) ToolBox {
	tools[def.Name()] = def
	return tools
}

func (tools ToolBox) Names() []string {
	names := []string{}
	for _, tool := range tools {
		names = append(names, tool.Name())
	}

	return names
}

func (tools ToolBox) Get(name string) (def *ToolDefinition, found bool) {
	def, found = tools[name]
	return
}

func (tools ToolBox) List() *genai.Tool {
	result := &genai.Tool{}
	for _, tool := range tools {
		result.FunctionDeclarations = append(result.FunctionDeclarations, tool.Tool.FunctionDeclarations...)
	}
	return result
}

func FormatFunctionCall(fc *genai.FunctionCall) string {
	buf := bytes.NewBufferString(fc.Name)
	if fc.ID != "" {
		fmt.Fprintf(buf, "@%s", fc.ID)
	}
	fmt.Fprintf(buf, "(%s)", AsJSON(fc.Args))
	return buf.String()
}
