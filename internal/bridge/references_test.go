package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const referenceFixture = `{"data":[{"cwd":"/work","skills":[{"name":"review","description":"Detailed review","enabled":true,"path":"/skills/review/SKILL.md","scope":"user","pluginId":"bundle","interface":{"displayName":"Review changes","shortDescription":"Inspect changes"}},{"name":"disabled","description":"Hidden","enabled":false,"path":"/skills/disabled/SKILL.md","scope":"repo"}],"errors":[{"message":"secret://token","path":"private"}]},{"cwd":"/other","skills":[{"name":"other","enabled":true,"path":"/other/SKILL.md"}]}]}`

func TestReferenceCatalogueProjection(t *testing.T) {
	if _, err := DecodeCommand([]byte(`{"id":1,"method":"references/list"}`)); err != nil {
		t.Fatal(err)
	}
	result, err := Handle(context.Background(), "/unused", Command{Method: "references/list", Params: json.RawMessage(`{"workspace":"/work","force_reload":true}`)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
		p := params.(map[string]any)
		if method != "skills/list" || p["forceReload"] != true {
			t.Fatalf("%s %v", method, p)
		}
		cwds, _ := json.Marshal(p["cwds"])
		if string(cwds) != `["/work"]` {
			t.Fatal(string(cwds))
		}
		return json.RawMessage(referenceFixture), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	for _, bad := range []string{"secret", "private", `"name":"other"`} {
		if strings.Contains(string(raw), bad) {
			t.Fatal(string(raw))
		}
	}
	for _, want := range []string{`"kind":"skill"`, `"title":"Review changes"`, `"enabled":false`, `"issues":1`, `"supported_kinds":["skill"]`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s: %s", want, raw)
		}
	}
}

func TestTurnReferenceNativeInputAndWorkspace(t *testing.T) {
	dispatched := 0
	_, err := Handle(context.Background(), "/work", Command{Method: "turn/start", Params: json.RawMessage(`{"thread_id":"one","text":"$review inspect this","references":[{"kind":"skill","name":"review","path":"/skills/review/SKILL.md"}]}`)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
		switch method {
		case "thread/read":
			return json.RawMessage(`{"thread":{"id":"one","cwd":"/work"}}`), nil
		case "skills/list":
			if params.(map[string]any)["forceReload"] != true {
				t.Fatal("stale validation")
			}
			return json.RawMessage(referenceFixture), nil
		case "turn/start":
			dispatched++
			raw, _ := json.Marshal(params.(map[string]any)["input"])
			if !strings.Contains(string(raw), `{"name":"review","path":"/skills/review/SKILL.md","type":"skill"}`) {
				t.Fatal(string(raw))
			}
			return json.RawMessage(`{}`), nil
		}
		return nil, errors.New("unexpected method")
	})
	if err != nil || dispatched != 1 {
		t.Fatalf("dispatch=%d err=%v", dispatched, err)
	}
}

func TestTurnReferenceFailsClosedBeforeDispatch(t *testing.T) {
	for _, refs := range []string{`null`, `{}`, `[{"kind":"mcp","name":"review","path":"/skills/review/SKILL.md"}]`, `[{"kind":"skill","name":"review","path":"/forged/SKILL.md"}]`, `[{"kind":"skill","name":"disabled","path":"/skills/disabled/SKILL.md"}]`, `[{"kind":"skill","name":"review","path":"/skills/review/SKILL.md","extra":true}]`} {
		t.Run(refs, func(t *testing.T) {
			_, err := Handle(context.Background(), "/work", Command{Method: "turn/start", Params: json.RawMessage(`{"thread_id":"one","text":"inspect","references":` + refs + `}`)}, func(_ context.Context, method string, _ any) (json.RawMessage, error) {
				if method == "turn/start" {
					t.Fatal("invalid reference dispatched")
				}
				if method == "skills/list" {
					return json.RawMessage(referenceFixture), nil
				}
				return json.RawMessage(`{"thread":{"id":"one","cwd":"/work"}}`), nil
			})
			if err == nil {
				t.Fatal("invalid reference accepted")
			}
		})
	}
}

func TestReferenceValidationBoundaries(t *testing.T) {
	ref := promptReference{Kind: "skill", Name: "review", Path: "/skills/review/SKILL.md"}
	fetch := func(_ string, _ any) (json.RawMessage, error) { return json.RawMessage(referenceFixture), nil }
	t.Run("duplicate selection is dispatched once", func(t *testing.T) {
		raw, _ := json.Marshal([]promptReference{ref, ref})
		input, err := appendReferences(fetch, "/work", nil, raw)
		if err != nil || len(input) != 1 {
			t.Fatalf("input=%v err=%v", input, err)
		}
	})
	t.Run("too many selections fail before discovery", func(t *testing.T) {
		raw, _ := json.Marshal([]promptReference{ref, ref, ref, ref, ref, ref, ref, ref, ref})
		_, err := appendReferences(func(string, any) (json.RawMessage, error) { t.Fatal("discovery for invalid request"); return nil, nil }, "/work", nil, raw)
		if err == nil {
			t.Fatal("over-limit selection accepted")
		}
	})
	t.Run("duplicate name with different path is ambiguous", func(t *testing.T) {
		other := ref
		other.Path = "/skills/other/SKILL.md"
		raw, _ := json.Marshal([]promptReference{ref, other})
		catalog := `{"data":[{"cwd":"/work","skills":[{"name":"review","path":"/skills/review/SKILL.md","enabled":true},{"name":"review","path":"/skills/other/SKILL.md","enabled":true}]}]}`
		_, err := appendReferences(func(string, any) (json.RawMessage, error) { return json.RawMessage(catalog), nil }, "/work", nil, raw)
		if err == nil {
			t.Fatal("ambiguous skill name accepted")
		}
	})
	t.Run("missing workspace cannot reuse another catalogue", func(t *testing.T) {
		if _, err := listReferences(fetch, "/missing", false); err == nil {
			t.Fatal("missing workspace accepted")
		}
	})
	for _, force := range []string{`null`, `"true"`, `1`} {
		t.Run("invalid force_reload "+force, func(t *testing.T) {
			_, err := Handle(context.Background(), "/work", Command{Method: "references/list", Params: json.RawMessage(`{"force_reload":` + force + `}`)}, func(context.Context, string, any) (json.RawMessage, error) {
				t.Fatal("invalid reload reached discovery")
				return nil, nil
			})
			if err == nil {
				t.Fatal("invalid reload accepted")
			}
		})
	}
}
