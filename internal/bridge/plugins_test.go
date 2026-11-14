package bridge

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const installedPluginFixture = `{"marketplaces":[{"name":"curated","plugins":[{"id":"review@curated","name":"review","installed":true,"enabled":true,"availability":"AVAILABLE","interface":{"displayName":"Review plugin","shortDescription":"Inspect changes"}},{"id":"disabled@curated","name":"disabled","installed":true,"enabled":false},{"id":"policy@curated","name":"policy","installed":true,"enabled":true,"availability":"DISABLED_BY_ADMIN"},{"id":"reason@curated","name":"reason","installed":true,"enabled":true,"disabledReason":"required_app_unavailable"},{"id":"store@curated","name":"store","installed":false,"enabled":true}]}],"marketplaceLoadErrors":[{"message":"secret://token"}]}`

func pluginFetch(method string, _ any) (json.RawMessage, error) {
	switch method {
	case "skills/list":
		return json.RawMessage(referenceFixture), nil
	case "plugin/installed":
		return json.RawMessage(installedPluginFixture), nil
	}
	return nil, errors.New("unexpected method")
}

func TestInstalledPluginCatalogue(t *testing.T) {
	catalog, err := listReferences(pluginFetch, "/work", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(catalog.SupportedKinds, ",") != "skill,plugin" || len(catalog.Entries) != 6 || catalog.Issues != 2 {
		t.Fatalf("%+v", catalog)
	}
	for _, entry := range catalog.Entries {
		if entry.Kind != "plugin" {
			continue
		}
		if entry.Enabled != (entry.Name == "review") {
			t.Fatalf("wrong availability: %+v", entry)
		}
		if entry.Name == "review" && (entry.Path != "plugin://review@curated" || entry.Marker != "@review" || entry.Title != "Review plugin") {
			t.Fatal(entry)
		}
	}
	raw, _ := json.Marshal(catalog)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "store@") {
		t.Fatal("uninstalled plugin or raw error leaked")
	}
}

func TestPluginReferencesUseNativeMentions(t *testing.T) {
	refs := json.RawMessage(`[{"kind":"skill","name":"review","path":"/skills/review/SKILL.md"},{"kind":"plugin","name":"review","path":"plugin://review@curated"},{"kind":"plugin","name":"review","path":"plugin://review@curated"}]`)
	input, err := appendReferences(pluginFetch, "/work", nil, refs)
	if err != nil || len(input) != 2 {
		t.Fatalf("input=%v err=%v", input, err)
	}
	if input[0]["type"] != "skill" || input[1]["type"] != "mention" || input[1]["path"] != "plugin://review@curated" {
		t.Fatal(input)
	}
	for _, name := range []string{"forged", "disabled", "policy", "reason", "store"} {
		raw, _ := json.Marshal([]promptReference{{Kind: "plugin", Name: name, Path: "plugin://" + name + "@curated"}})
		if _, err := appendReferences(pluginFetch, "/work", nil, raw); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
}

func TestPluginDiscoveryCompatibilityAndBounds(t *testing.T) {
	for _, response := range []string{`{}`, `null`, `{"marketplaces":null}`, `invalid`} {
		catalog, err := listReferences(func(method string, params any) (json.RawMessage, error) {
			if method == "plugin/installed" {
				return json.RawMessage(response), nil
			}
			return pluginFetch(method, params)
		}, "/work", false)
		if err != nil || len(catalog.Entries) != 2 || len(catalog.SupportedKinds) != 1 || catalog.Issues != 2 {
			t.Fatalf("%+v %v", catalog, err)
		}
	}
	for _, path := range []string{"plugin://", "/tmp/plugin", "plugin://bad/name", "plugin://bad\nname", "https://review@curated"} {
		if validReference(promptReference{Kind: "plugin", Name: "review", Path: path}) {
			t.Fatal(path)
		}
	}
	catalog, err := listReferences(func(method string, params any) (json.RawMessage, error) {
		if method == "skills/list" {
			return pluginFetch(method, params)
		}
		return json.RawMessage(`{"marketplaces":[{"plugins":[{"id":"a@b","name":"a","installed":true,"enabled":true},{"id":"a@b","name":"a","installed":true,"enabled":true}]}]}`), nil
	}, "/work", false)
	if err != nil || len(catalog.Entries) != 3 {
		t.Fatal("duplicate plugin id")
	}
}
