package bridge

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
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

func validProjectText(s string) bool {
	return strings.TrimSpace(s) != "" && len(s) <= 256 && !strings.ContainsFunc(s, unicode.IsControl)
}

func createProject(params map[string]json.RawMessage, fetch func(string, any) (json.RawMessage, error)) (any, error) {
	var name, root, key string
	if json.Unmarshal(params["name"], &name) != nil || !validProjectText(name) ||
		json.Unmarshal(params["idempotency_key"], &key) != nil || !validProjectText(key) ||
		json.Unmarshal(params["workspace"], &root) != nil || !filepath.IsAbs(root) {
		return nil, errors.New("project requires name, absolute workspace and idempotency_key")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, errors.New("project folder is unavailable")
	}
	raw, err := fetch("project/create", map[string]any{"name": strings.TrimSpace(name), "roots": []map[string]string{{"path": root}}, "idempotencyKey": key})
	if err != nil {
		return nil, err
	}
	var result struct {
		Project projectEntry `json:"project"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Project.ID == "" || len(result.Project.Roots) != 1 || result.Project.Roots[0].Path != root {
		return nil, errors.New("invalid project response; refresh projects before retrying")
	}
	return result, nil
}
