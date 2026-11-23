package bridge

import (
	"encoding/json"
	"errors"
	"strings"
)

type permissionProfile struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Allowed     bool   `json:"allowed"`
}

func sessionPermissions(p map[string]json.RawMessage, cwd string, fetch func(string, any) (json.RawMessage, error)) (any, error) {
	var id, selected string
	if json.Unmarshal(p["thread_id"], &id) != nil || !validPermissionID(id) {
		return nil, errors.New("invalid thread id")
	}
	if value, ok := p["profile"]; ok && (json.Unmarshal(value, &selected) != nil || !validPermissionID(selected)) {
		return nil, errors.New("invalid permission profile")
	}
	raw, err := fetch("thread/read", map[string]any{"threadId": id, "includeTurns": false})
	if err != nil {
		return nil, err
	}
	var metadata struct {
		Thread Thread `json:"thread"`
	}
	if json.Unmarshal(raw, &metadata) != nil || metadata.Thread.ID != id || metadata.Thread.Cwd != cwd {
		return nil, errors.New("thread outside workspace or unavailable")
	}
	if metadata.Thread.Status != nil && metadata.Thread.Status.Type == "notLoaded" {
		return nil, ErrSessionNotOwned
	}
	if _, err := fetch("modex/thread/status", map[string]any{"threadId": id}); err != nil {
		return nil, err
	}
	raw, err = fetch("permissionProfile/list", map[string]any{"cwd": cwd})
	if err != nil {
		return nil, err
	}
	var catalog struct {
		Data []permissionProfile `json:"data"`
	}
	if json.Unmarshal(raw, &catalog) != nil || catalog.Data == nil {
		return nil, errors.New("invalid permission catalog")
	}
	if selected == "" {
		profiles := make([]permissionProfile, 0, len(catalog.Data))
		for _, profile := range catalog.Data {
			if !validPermissionID(profile.ID) {
				continue
			}
			profile.Description = safePreview(profile.Description)
			profiles = append(profiles, profile)
		}
		return map[string]any{"profiles": profiles}, nil
	}
	for _, profile := range catalog.Data {
		if profile.ID != selected || !profile.Allowed {
			continue
		}
		policy := "on-request"
		if selected == ":danger-full-access" {
			policy = "never"
		}
		return fetch("modex/thread/permissions", map[string]any{
			"threadId": id, "permissions": selected, "approvalPolicy": policy, "approvalsReviewer": "user",
		})
	}
	return nil, errors.New("permission profile unavailable")
}

func validPermissionID(id string) bool {
	return strings.TrimSpace(id) == id && id != "" && len(id) <= 256 && !strings.ContainsAny(id, "\r\n\x00")
}
