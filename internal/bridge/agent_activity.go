package bridge

import (
	"path"
	"strings"
	"unicode"
)

type nativeAgentActivity struct {
	Kind    string   `json:"kind"`
	AgentID string   `json:"agentThreadId"`
	Path    string   `json:"agentPath"`
	Tool    string   `json:"tool"`
	Targets []string `json:"receiverThreadIds"`
	States  map[string]struct {
		Status string `json:"status"`
	} `json:"agentsStates"`
}

type agentActivityTarget struct {
	ID     string `json:"id"`
	Label  string `json:"label,omitempty"`
	Status string `json:"status,omitempty"`
}

type agentActivity struct {
	ID      string                `json:"id"`
	Type    string                `json:"type"`
	Action  string                `json:"action"`
	Targets []agentActivityTarget `json:"targets"`
}

func isAgentActivity(kind string) bool {
	return kind == "subAgentActivity" || kind == "collabAgentToolCall"
}

func (item nativeAgentActivity) project(id, kind, status string) agentActivity {
	activity := agentActivity{ID: id, Type: "agentActivity", Targets: []agentActivityTarget{}}
	if kind == "subAgentActivity" {
		target := agentActivityTarget{ID: item.AgentID}
		switch item.Kind {
		case "started":
			activity.Action, target.Status = "started", "running"
		case "interacted":
			activity.Action = "interacted"
		case "interrupted", "completed":
			activity.Action, target.Status = item.Kind, item.Kind
		}
		label := path.Base(strings.TrimSpace(item.Path))
		if label != "." && label != "/" && len(label) <= 96 && !strings.ContainsFunc(label, unicode.IsControl) {
			target.Label = label
		}
		if validAgentID(target.ID) && activity.Action != "" {
			activity.Targets = append(activity.Targets, target)
		}
		return activity
	}

	switch item.Tool {
	case "spawnAgent":
		activity.Action = "started"
	case "sendInput", "followupTask":
		activity.Action = "assigned"
	case "sendMessage":
		activity.Action = "interacted"
	case "resumeAgent":
		activity.Action = "resumed"
	case "wait":
		activity.Action = "waited"
	case "interruptAgent", "closeAgent":
		activity.Action = "stopRequested"
	default:
		return activity
	}
	if status != "completed" {
		return activity
	}
	seen := make(map[string]bool)
	for _, agentID := range item.Targets {
		if len(activity.Targets) == 64 {
			break
		}
		if !validAgentID(agentID) || seen[agentID] {
			continue
		}
		seen[agentID] = true
		state := item.States[agentID].Status
		switch state {
		case "pendingInit", "running", "interrupted", "completed", "errored", "shutdown", "notFound":
		default:
			state = ""
		}
		activity.Targets = append(activity.Targets, agentActivityTarget{ID: agentID, Status: state})
	}
	return activity
}

func validAgentID(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsFunc(id, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	})
}
