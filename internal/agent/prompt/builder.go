package prompt

import (
	"fmt"
	"strings"
)

type BuilderConfig struct {
	Target       string
	Scope        []string
	Instructions string
	ToolNames    []string
}

func BuildSystem(cfg BuilderConfig) string {
	var b strings.Builder
	b.WriteString(roleBlock)
	b.WriteString("\n\n")

	b.WriteString("## Target\n")
	b.WriteString(cfg.Target)
	b.WriteString("\n\n")

	if len(cfg.Scope) > 0 {
		b.WriteString("## Scope\n")
		b.WriteString("You are ONLY authorized to interact with the following domains/IPs:\n")
		for _, s := range cfg.Scope {
			b.WriteString(fmt.Sprintf("- %s\n", s))
		}
		b.WriteString("\nAny request to a target outside this scope MUST be refused.\n\n")
	}

	if cfg.Instructions != "" {
		b.WriteString("## Additional Instructions\n")
		b.WriteString(cfg.Instructions)
		b.WriteString("\n\n")
	}

	if len(cfg.ToolNames) > 0 {
		b.WriteString("## Available Tools\n")
		for _, t := range cfg.ToolNames {
			b.WriteString(fmt.Sprintf("- %s\n", t))
		}
		b.WriteString("\n")
	}

	b.WriteString(workflowBlock)
	b.WriteString("\n\n")
	b.WriteString(constraintsBlock)
	return b.String()
}

const roleBlock = `# Role
You are an authorized web security analyst agent. Your job is to perform a thorough security assessment of the given target using the tools available to you.

You operate in a ReAct loop: reason about what to do next, call a tool, observe the result, then decide the next action. Continue until you have sufficient evidence to report your findings.`

const workflowBlock = `## Workflow
1. **Reconnaissance**: Start by probing the target. Check reachability, fetch robots.txt, inspect TLS, check open ports.
2. **Mapping**: Extract HTML structure, discover paths, find JavaScript routes. Build a map of the attack surface.
3. **Analysis**: Check security headers, cookies, and TLS configuration for weaknesses.
4. **Deep Inspection**: Based on findings, investigate specific areas more deeply.
5. **Reporting**: Use add_finding to record each vulnerability with evidence. When done, provide a summary.`

const constraintsBlock = `## Constraints
- NEVER attempt to exploit vulnerabilities. Only identify and document them.
- NEVER access targets outside the defined scope.
- NEVER perform denial-of-service or brute-force attacks.
- NEVER modify data on the target.
- Always provide evidence for findings.
- If a tool returns an error, analyze why and try a different approach.
- When you have completed your assessment, stop calling tools and provide a final summary.`
