package bridge

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Keep Desktop-owned project identity and roots without exposing arbitrary metadata.
type projectEntry struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Roots []struct {
		Path string `json:"path"`
	} `json:"roots"`
}

// Desktop versions predating canonical membership keep explicit assignments locally.
func desktopMembership(raw json.RawMessage, state []byte, home string) json.RawMessage {
	var legacy struct {
		Assignments map[string]struct {
			Kind string `json:"projectKind"`
			ID   string `json:"projectId"`
		} `json:"thread-project-assignments"`
		Projectless []string                     `json:"projectless-thread-ids"`
		IDs         map[string]map[string]string `json:"app-server-project-id-by-legacy-project-id-by-host"`
	}
	var page map[string]json.RawMessage
	var threads []map[string]json.RawMessage
	if json.Unmarshal(state, &legacy) != nil || json.Unmarshal(raw, &page) != nil || json.Unmarshal(page["data"], &threads) != nil {
		return raw
	}
	excluded := make(map[string]bool, len(legacy.Projectless))
	for _, id := range legacy.Projectless {
		excluded[id] = true
	}
	changed := false
	for _, thread := range threads {
		var id, project string
		if json.Unmarshal(thread["id"], &id) != nil {
			continue
		}
		_ = json.Unmarshal(thread["projectId"], &project)
		assignment := legacy.Assignments[id]
		mapped := legacy.IDs["local:"+home][assignment.ID]
		if project == "" && excluded[id] {
			thread["projectless"] = json.RawMessage(`true`)
			changed = true
			continue
		}
		if project != "" || assignment.Kind != "local" || mapped == "" {
			continue
		}
		thread["projectId"], _ = json.Marshal(mapped)
		changed = true
	}
	if !changed {
		return raw
	}
	page["data"], _ = json.Marshal(threads)
	result, err := json.Marshal(page)
	if err != nil {
		return raw
	}
	return result
}

func (p *Process) desktopMembership(raw json.RawMessage) json.RawMessage {
	home := ""
	for _, entry := range p.cmd.Environ() {
		if value, ok := strings.CutPrefix(entry, "CODEX_HOME="); ok {
			home = value
		}
	}
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return raw
		}
		home = filepath.Join(userHome, ".codex")
	}
	f, err := os.Open(filepath.Join(home, ".codex-global-state.json"))
	if err != nil {
		return raw
	}
	defer f.Close()
	const maxState = 8 * 1024 * 1024
	state, err := io.ReadAll(io.LimitReader(f, maxState+1))
	if err != nil || len(state) > maxState {
		return raw
	}
	return desktopMembership(raw, state, home)
}
