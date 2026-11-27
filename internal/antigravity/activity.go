package antigravity

import (
	"crypto/sha256"
	"fmt"

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
	Action  string             `json:"action"`
	Targets []invocationTarget `json:"targets"`
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
