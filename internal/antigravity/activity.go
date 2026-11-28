package antigravity

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/warmdev/modex/internal/acp"
)

type invocationTarget struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

type invocationItem struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Action  string             `json:"action,omitempty"`
	Targets []invocationTarget `json:"targets,omitempty"`
	Status  string             `json:"status,omitempty"`
	Changes []fileChange       `json:"changes,omitempty"`
}

// ACP reports the invocation tool's outcome, which may precede the child's outcome.
func (c *chat) updateInvocation(update acp.Update) {
	if !c.loading && (c.active == "" || c.active != update.TurnID) {
		return
	}
	if c.loading {
		if index, found := c.replayCalls[update.ToolCallID]; found {
			if status := invocationStatus(update.Status); invocationEnded(status) {
				item := c.messages[index].Activity
				if !invocationEnded(item.Targets[0].Status) {
					item.Targets[0].Status = status
					item.ID = item.Targets[0].ID + ":" + status
					c.messages[index].ID = item.ID
				}
			}
			return
		}
		if update.Kind != "tool_call" || !update.Subagent {
			return
		}
		if c.replayCalls == nil {
			c.replayCalls = make(map[string]int)
		}
		c.replayCalls[update.ToolCallID] = len(c.messages)
		target := newInvocationTarget(update.Role, "", update.ToolCallID, len(c.messages))
		target.Status = invocationStatus(update.Status)
		if target.Status == "pendingInit" || target.Status == "running" {
			target.Status = "unknown"
		}
		c.emitInvocation("", "completed", target)
		return
	}
	target, found := c.calls[update.ToolCallID]
	if !found {
		if update.Kind != "tool_call" || !update.Subagent || len(c.calls) >= 128 {
			return
		}
		if c.calls == nil {
			c.calls = make(map[string]invocationTarget)
		}
		target = newInvocationTarget(update.Role, update.TurnID, update.ToolCallID, len(c.messages))
	}
	status := invocationStatus(update.Status)
	if status == target.Status || (found && invocationEnded(target.Status)) || (found && status == "unknown") {
		return
	}
	target.Status = status
	c.calls[update.ToolCallID] = target
	action := "started"
	if invocationEnded(status) {
		action = "completed"
	}
	c.emitInvocation(update.TurnID, action, target)
}

func newInvocationTarget(role, turn, tool string, position int) invocationTarget {
	if role == "" {
		role = "Antigravity subagent"
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", turn, tool, position)))
	return invocationTarget{ID: fmt.Sprintf("agy-call:%x", sum[:12]), Label: role}
}

func invocationStatus(status string) string {
	switch status {
	case "pending":
		return "pendingInit"
	case "in_progress":
		return "running"
	case "completed":
		return "invocationCompleted"
	case "failed":
		return "errored"
	case "cancelled":
		return "interrupted"
	default:
		return "unknown"
	}
}

func invocationEnded(status string) bool {
	return status == "invocationCompleted" || status == "errored" || status == "interrupted"
}

func (c *chat) emitInvocation(turn, action string, target invocationTarget) {
	id := target.ID + ":" + target.Status
	item := invocationItem{ID: id, Type: "agentActivity", Action: action, Targets: []invocationTarget{target}}
	c.messages = append(c.messages, message{ID: id, Role: "agent_activity", Turn: turn, Activity: &item})
	if !c.loading {
		event := "item/started"
		if action == "completed" {
			event = "item/completed"
		}
		emitEvent(c.emit, event, map[string]any{"threadId": c.ID, "turnId": turn, "item": item})
	}
}

type commandState struct {
	index    int
	terminal bool
}

func (c *chat) updateCommand(u acp.Update) {
	if !c.loading && (c.active == "" || c.active != u.TurnID) {
		return
	}
	if u.ForeignTool {
		return
	}
	state, found := c.commandCalls[u.ToolCallID]
	if !found {
		if u.Kind != "tool_call" || !u.CommandTool || u.Command == "" || len(c.commandCalls) >= 128 {
			return
		}
		if c.commandCalls == nil {
			c.commandCalls = make(map[string]commandState)
		}
		id := newInvocationTarget("", u.TurnID, u.ToolCallID, len(c.messages)).ID
		if !c.loading {
			c.segment++
		}
		state.index = len(c.messages)
		c.messages = append(c.messages, message{ID: id, Role: "command", Turn: u.TurnID, Command: u.Command, Status: "inProgress"})
	}
	if state.terminal {
		return
	}
	m := &c.messages[state.index]
	if u.Output != "" {
		m.Text = u.Output
	}
	if u.ExitCode != nil {
		m.ExitCode = u.ExitCode
	}
	switch u.Status {
	case "completed":
		m.Status = "completed"
		state.terminal = true
	case "failed":
		m.Status = "failed"
		state.terminal = true
	case "cancelled":
		m.Status = "interrupted"
		state.terminal = true
	}
	c.commandCalls[u.ToolCallID] = state
	if c.loading {
		if !state.terminal {
			m.Status = "unknown"
		}
		return
	}
	if !found && !state.terminal {
		c.emitCommand(*m, "item/started")
	}
	if state.terminal {
		c.emitCommand(*m, "item/completed")
	}
}

func (c *chat) emitCommand(m message, event string) {
	item := map[string]any{"id": m.ID, "type": "commandExecution", "command": m.Command, "aggregatedOutput": m.Text, "status": m.Status, "exitCode": m.ExitCode}
	emitEvent(c.emit, event, map[string]any{"threadId": c.ID, "turnId": m.Turn, "item": item})
}

type fileChange struct {
	Path string `json:"path"`
	Diff string `json:"diff"`
}

func (c *chat) updateDiff(u acp.Update) {
	if (!c.loading && (c.active == "" || c.active != u.TurnID)) || u.Status != "completed" || len(u.Diffs) == 0 || c.diffCalls[u.ToolCallID] || len(c.diffCalls) >= 128 {
		return
	}
	if c.diffCalls == nil {
		c.diffCalls = make(map[string]bool)
	}
	c.diffCalls[u.ToolCallID] = true
	id := newInvocationTarget("", u.TurnID, u.ToolCallID, len(c.messages)).ID
	item := invocationItem{ID: id, Type: "fileChange", Status: "completed"}
	remaining := 60 << 10
	omitted := u.DiffsTruncated
	for _, d := range u.Diffs {
		if remaining <= len(d.Path)+32 {
			omitted = true
			break
		}
		patch := displayDiffLimit(d.OldText, d.NewText, remaining-len(d.Path)-32)
		item.Changes = append(item.Changes, fileChange{Path: d.Path, Diff: patch})
		remaining -= len(d.Path) + len(patch)
	}

	if omitted && len(item.Changes) > 0 {
		item.Changes[len(item.Changes)-1].Diff += "[Additional diffs truncated]\n"
	}
	if !c.loading {
		c.segment++
	}
	c.messages = append(c.messages, message{ID: id, Role: "file_change", Turn: u.TurnID, Status: "completed", Activity: &item})
	if !c.loading {
		emitEvent(c.emit, "item/completed", map[string]any{"threadId": c.ID, "turnId": u.TurnID, "item": item})
	}
}

func displayDiffLimit(old, newText string, limit int) string {
	// Trim common whole lines without allocating a slice for every line.
	for old != "" && newText != "" {
		a, b := strings.IndexByte(old, '\n'), strings.IndexByte(newText, '\n')
		if a < 0 || b < 0 || old[:a+1] != newText[:b+1] {
			break
		}
		old = old[a+1:]
		newText = newText[b+1:]
	}
	for old != "" && newText != "" {
		a, b := lastLineStart(old), lastLineStart(newText)
		if old[a:] != newText[b:] {
			break
		}
		old = old[:a]
		newText = newText[:b]
	}
	var out strings.Builder
	appendLines := func(s, prefix string) bool {
		for s != "" {
			i := strings.IndexByte(s, '\n')
			var line string
			if i < 0 {
				line = s
				s = ""
			} else {
				line = s[:i]
				s = s[i+1:]
			}
			if out.Len()+len(line)+2+len("[Diff truncated]\n") > limit {
				out.WriteString("[Diff truncated]\n")
				return true
			}
			out.WriteString(prefix)
			out.WriteString(line)
			out.WriteByte('\n')
		}
		return false
	}
	if !appendLines(old, "-") {
		appendLines(newText, "+")
	}
	return out.String()
}
func lastLineStart(s string) int {
	end := len(s)
	if end > 0 && s[end-1] == '\n' {
		end--
	}
	return strings.LastIndexByte(s[:end], '\n') + 1
}
