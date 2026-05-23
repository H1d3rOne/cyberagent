package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/core/config"
)

func (r *Registry) RegisterCustomTools(tools []config.CustomTool) {
	for _, t := range tools {
		tool := t
		timeout := time.Duration(tool.Timeout) * time.Second
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		var inputSchema json.RawMessage
		if tool.InputSchema != "" {
			inputSchema = json.RawMessage(tool.InputSchema)
		} else {
			inputSchema = json.RawMessage(`{"type":"object","properties":{"input":{"type":"string","description":"Input to pass to the tool"}},"required":["input"]}`)
		}

		r.Register(tool.Name, RegisteredTool{
			Def: llm.ToolDef{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: inputSchema,
			},
			Execute: makeCustomExecutor(tool, timeout),
		})
	}
}

func makeCustomExecutor(tool config.CustomTool, timeout time.Duration) ToolFunc {
	return func(ctx context.Context, input json.RawMessage) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		args := make([]string, len(tool.Args))
		copy(args, tool.Args)

		cmd := exec.CommandContext(ctx, tool.Command, args...)

		for k, v := range tool.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		cmd.Stdin = bytes.NewReader(input)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return "", fmt.Errorf("custom tool %s timed out after %s", tool.Name, timeout)
			}
			if stderr.Len() > 0 {
				return "", fmt.Errorf("custom tool %s: %s", tool.Name, stderr.String())
			}
			return "", fmt.Errorf("custom tool %s: %w", tool.Name, err)
		}

		return stdout.String(), nil
	}
}
