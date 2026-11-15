package bridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type promptReference struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type referenceEntry struct {
	promptReference
	Marker      string `json:"marker"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
	PluginID    string `json:"plugin_id,omitempty"`
}

type referenceCatalog struct {
	Entries        []referenceEntry `json:"entries"`
	SupportedKinds []string         `json:"supported_kinds"`
	Issues         int              `json:"issues"`
}

func validReference(r promptReference) bool {
	return validReferencePath(r) && r.Name != "" && len(r.Name) <= 256 &&
		!strings.ContainsAny(r.Name, "@$`") && strings.IndexFunc(r.Name, func(c rune) bool { return unicode.IsSpace(c) || unicode.IsControl(c) }) == -1 &&
		len(r.Path) <= 32768 && !strings.ContainsAny(r.Path, "\r\n\x00")
}

func validReferencePath(r promptReference) bool {
	if r.Kind == "skill" {
		return filepath.IsAbs(r.Path)
	}
	if r.Kind != "plugin" || !strings.HasPrefix(r.Path, "plugin://") {
		return false
	}
	id := strings.TrimPrefix(r.Path, "plugin://")
	return id != "" && strings.IndexFunc(id, func(c rune) bool {
		return !(unicode.IsLetter(c) || unicode.IsDigit(c) || strings.ContainsRune("-_.@", c))
	}) == -1
}

// Installed discovery is optional so older Codex hosts can still expose skills.
func appendInstalledPlugins(fetch func(string, any) (json.RawMessage, error), cwd string, catalog *referenceCatalog) {
	raw, err := fetch("plugin/installed", map[string]any{"cwds": []string{cwd}})
	if err != nil {
		catalog.Issues++
		return
	}
	var result struct {
		Marketplaces []struct {
			Name    string `json:"name"`
			Plugins []struct {
				ID             string  `json:"id"`
				Name           string  `json:"name"`
				Installed      bool    `json:"installed"`
				Enabled        bool    `json:"enabled"`
				Availability   string  `json:"availability"`
				DisabledReason *string `json:"disabledReason"`
				Interface      *struct {
					DisplayName      string `json:"displayName"`
					ShortDescription string `json:"shortDescription"`
				} `json:"interface"`
			} `json:"plugins"`
		} `json:"marketplaces"`
		Errors []json.RawMessage `json:"marketplaceLoadErrors"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Marketplaces == nil {
		catalog.Issues++
		return
	}
	catalog.SupportedKinds = append(catalog.SupportedKinds, "plugin")
	catalog.Issues += len(result.Errors)
	seen := map[string]bool{}
	for _, marketplace := range result.Marketplaces {
		for _, plugin := range marketplace.Plugins {
			if !plugin.Installed {
				continue
			}
			ref := promptReference{Kind: "plugin", Name: plugin.Name, Path: "plugin://" + plugin.ID}
			if !validReference(ref) || len(catalog.Entries) >= 2048 {
				catalog.Issues++
				continue
			}
			if seen[plugin.ID] {
				continue
			}
			seen[plugin.ID] = true
			title, description := plugin.Name, ""
			if plugin.Interface != nil {
				if plugin.Interface.DisplayName != "" {
					title = plugin.Interface.DisplayName
				}
				description = plugin.Interface.ShortDescription
			}
			enabled := plugin.Enabled && plugin.DisabledReason == nil && (plugin.Availability == "" || plugin.Availability == "AVAILABLE")
			catalog.Entries = append(catalog.Entries, referenceEntry{promptReference: ref, Marker: "@" + plugin.Name, Title: title, Description: description, Scope: marketplace.Name, Enabled: enabled, PluginID: plugin.ID})
		}
	}
}

func listReferences(fetch func(string, any) (json.RawMessage, error), cwd string, force bool) (referenceCatalog, error) {
	catalog := referenceCatalog{Entries: []referenceEntry{}, SupportedKinds: []string{"skill"}}
	raw, err := fetch("skills/list", map[string]any{"cwds": []string{cwd}, "forceReload": force})
	if err != nil {
		return catalog, err
	}
	var result struct {
		Data []struct {
			Cwd    string `json:"cwd"`
			Skills []struct {
				Name             string `json:"name"`
				Path             string `json:"path"`
				Description      string `json:"description"`
				ShortDescription string `json:"shortDescription"`
				Scope            string `json:"scope"`
				Enabled          bool   `json:"enabled"`
				PluginID         string `json:"pluginId"`
				Interface        *struct {
					DisplayName      string `json:"displayName"`
					ShortDescription string `json:"shortDescription"`
				} `json:"interface"`
			} `json:"skills"`
			Errors []json.RawMessage `json:"errors"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Data == nil {
		return catalog, errors.New("invalid skill catalogue")
	}
	found := false
	seen := map[promptReference]bool{}
	for _, group := range result.Data {
		if group.Cwd != cwd {
			continue
		}
		found = true
		catalog.Issues += len(group.Errors)
		for _, skill := range group.Skills {
			ref := promptReference{Kind: "skill", Name: skill.Name, Path: skill.Path}
			if !validReference(ref) || len(catalog.Entries) >= 2048 {
				catalog.Issues++
				continue
			}
			if seen[ref] {
				continue
			}
			seen[ref] = true
			title, description := skill.Name, skill.Description
			if skill.ShortDescription != "" {
				description = skill.ShortDescription
			}
			if skill.Interface != nil {
				if skill.Interface.DisplayName != "" {
					title = skill.Interface.DisplayName
				}
				if skill.Interface.ShortDescription != "" {
					description = skill.Interface.ShortDescription
				}
			}
			catalog.Entries = append(catalog.Entries, referenceEntry{promptReference: ref, Marker: "$" + skill.Name, Title: title, Description: description, Scope: skill.Scope, Enabled: skill.Enabled, PluginID: skill.PluginID})
		}
	}
	if !found {
		return catalog, errors.New("skill catalogue did not include the requested workspace")
	}
	appendInstalledPlugins(fetch, cwd, &catalog)
	sort.SliceStable(catalog.Entries, func(i, j int) bool {
		a, b := catalog.Entries[i], catalog.Entries[j]
		if a.Name == b.Name {
			return a.Path < b.Path
		}
		return a.Name < b.Name
	})
	return catalog, nil
}

func appendReferences(fetch func(string, any) (json.RawMessage, error), cwd string, input []map[string]string, raw json.RawMessage) ([]map[string]string, error) {
	if raw == nil {
		return input, nil
	}
	var references []promptReference
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&references) != nil || references == nil || len(references) > 8 {
		return nil, errors.New("invalid prompt references; select up to 8 references")
	}
	if len(references) == 0 {
		return input, nil
	}
	for _, ref := range references {
		if !validReference(ref) {
			return nil, errors.New("unsupported or invalid prompt reference")
		}
	}
	// Recheck Codex discovery rather than accepting arbitrary paths from the phone.
	catalog, err := listReferences(fetch, cwd, true)
	if err != nil {
		return nil, err
	}
	available := map[promptReference]bool{}
	for _, entry := range catalog.Entries {
		if entry.Enabled {
			available[entry.promptReference] = true
		}
	}
	names := map[string]string{}
	for _, ref := range references {
		if !available[ref] {
			return nil, errors.New("prompt reference is unavailable or disabled; refresh references")
		}
		if previous, exists := names[ref.Kind+":"+ref.Name]; exists {
			if previous != ref.Path {
				return nil, errors.New("select only one reference of each kind with the same name")
			}
			continue
		}
		names[ref.Kind+":"+ref.Name] = ref.Path
		inputType := "skill"
		if ref.Kind == "plugin" {
			inputType = "mention"
		}
		input = append(input, map[string]string{"type": inputType, "name": ref.Name, "path": ref.Path})
	}
	return input, nil
}
