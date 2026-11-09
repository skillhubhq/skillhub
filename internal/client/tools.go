package client

import (
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed tools.json
var toolCatalog []byte

type ToolAdapter struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ProjectDirectory string `json:"project_directory"`
	GlobalDirectory  string `json:"global_directory"`
}

func Tools() []ToolAdapter {
	var result []ToolAdapter
	if err := json.Unmarshal(toolCatalog, &result); err != nil {
		panic(err)
	}
	return result
}
func toolDirectory(tool, scope, name string) (string, error) {
	if tool == "claude-code" && (name == "synced" || name == "anthropic-skills") {
		return "", errors.New("skill name is reserved by Claude Code")
	}
	for _, adapter := range Tools() {
		if adapter.ID == tool {
			if scope == "global" {
				return adapter.GlobalDirectory, nil
			}
			return adapter.ProjectDirectory, nil
		}
	}
	return "", errors.New("unsupported tool; use skillhub tools to list supported tools")
}
