package compaction

import (
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const SummarizationSystemPrompt = "You are a context summarization assistant. Output plain text only. Do not continue the conversation."

const SummarizationPrompt = `The conversation above is historical context to summarize for another coding agent.

Use this exact structure:

## Goal
[What the user is trying to accomplish]

## Constraints & Preferences
- [Any explicit constraints or preferences]

## Progress
### Done
- [x] [Completed items]

### In Progress
- [ ] [Current work]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [Ordered next actions]

## Critical Context
- [Important details needed to continue]

Keep it concise. Preserve exact file paths, function names, and important error messages.`

func BuildSummarizationInput(msgs []op.Message) string {
	conversation := strings.TrimSpace(op.SerializeMessagesForSummary(msgs))
	var b strings.Builder
	b.WriteString("<conversation>\n")
	b.WriteString(conversation)
	b.WriteString("\n</conversation>\n\n")
	b.WriteString(SummarizationPrompt)
	return b.String()
}
