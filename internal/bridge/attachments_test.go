package bridge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTurnAttachmentsNativeImageAndFileReferences(t *testing.T) {
	root := t.TempDir()
	image := filepath.Join(root, "photo.png")
	document := filepath.Join(root, "notes.txt")
	// DetectContentType needs a PNG signature; image decoding is Codex's responsibility.
	if err := os.WriteFile(image, []byte("\x89PNG\r\n\x1a\nfixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(document, []byte("notes"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Inspect these", ""} {
		raw, _ := json.Marshal(map[string]any{"thread_id": "t", "text": text, "attachments": []map[string]string{{"path": image, "kind": "image"}, {"path": document, "kind": "file"}}})
		_, err := Handle(context.Background(), root, Command{Method: "turn/start", Params: raw}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
			if method == "thread/read" {
				b, _ := json.Marshal(map[string]any{"thread": map[string]string{"id": "t", "cwd": root}})
				return b, nil
			}
			b, _ := json.Marshal(params)
			var p struct {
				Input []struct{ Type, Text, Path string } `json:"input"`
			}
			_ = json.Unmarshal(b, &p)
			foundImage, foundFile := false, false
			for _, part := range p.Input {
				if part.Type == "localImage" && part.Path == image {
					foundImage = true
				}
				if part.Type == "text" && strings.Contains(part.Text, document) {
					foundFile = true
				}
			}
			if !foundImage || !foundFile {
				t.Fatalf("attachment mapping: %s", b)
			}
			return json.RawMessage(`{"turn":{"id":"ok"}}`), nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTurnRejectsInvalidAttachmentsBeforeUpstream(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	for _, attachments := range []any{nil, []any{}, []map[string]string{{"path": "relative", "kind": "file"}}, []map[string]string{{"path": root, "kind": "file"}}, []map[string]string{{"path": missing, "kind": "file"}}, []map[string]string{{"path": missing, "kind": "unknown"}}} {
		raw, _ := json.Marshal(map[string]any{"thread_id": "t", "text": "hi", "attachments": attachments})
		_, err := Handle(context.Background(), root, Command{Method: "turn/start", Params: raw}, func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("invalid attachment reached upstream")
			return nil, nil
		})
		if err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestAttachmentSizeTypeAndCountBounds(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(value any) {
		t.Helper()
		b, _ := json.Marshal(value)
		if _, err := turnInput("hi", b); err == nil {
			t.Fatalf("accepted invalid attachments: %s", b)
		}
	}
	run([]map[string]string{{"path": path, "kind": "image"}})
	run([]map[string]string{{"path": path, "kind": "file", "unknown": "bad"}})
	many := make([]map[string]string, 9)
	for i := range many {
		many[i] = map[string]string{"path": path, "kind": "file"}
	}
	run(many)
	if err := os.Truncate(path, maxAttachmentBytes+1); err != nil {
		t.Fatal(err)
	}
	run([]map[string]string{{"path": path, "kind": "file"}})
}

func TestImageOnlyHistoryRemainsVisible(t *testing.T) {
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"t","include_turns":true}`)}, func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`{"thread":{"id":"t","cwd":"/work","turns":[{"items":[{"id":"u","type":"userMessage","content":[{"type":"localImage","path":"/private/photo.png"}]}]}]}}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(result)
	if !strings.Contains(string(b), "photo.png") {
		t.Fatal(string(b))
	}
}

func TestAttachmentHistoryIncludesPreviewPaths(t *testing.T) {
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"t","include_turns":true}`)}, func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`{"thread":{"id":"t","cwd":"/work","turns":[{"items":[{"id":"u","type":"userMessage","content":[{"type":"text","text":"look"},{"type":"localImage","path":"/private/photo.png"},{"type":"text","text":"Attached files selected by the user (JSON paths):\n[{\"name\":\"notes.txt\",\"path\":\"/private/notes.txt\"}]"}]}]}]}}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	var decoded struct {
		Messages []struct {
			Attachments []attachmentInput `json:"attachments"`
			Text        string            `json:"text"`
			DisplayText string            `json:"display_text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Messages) != 1 || len(decoded.Messages[0].Attachments) != 2 {
		t.Fatalf("missing preview metadata: %s", encoded)
	}
	if decoded.Messages[0].Attachments[0].Path != "/private/photo.png" || decoded.Messages[0].Attachments[1].Kind != "file" {
		t.Fatalf("wrong metadata: %s", encoded)
	}
	if decoded.Messages[0].DisplayText != "look" {
		t.Fatalf("user display text changed: %s", encoded)
	}
	if strings.Contains(decoded.Messages[0].Text, "JSON paths") {
		t.Fatalf("raw file reference leaked into display: %s", encoded)
	}
}

func TestHistoryFileReferencesRejectOtherContent(t *testing.T) {
	for _, content := range []string{"hello", "Attached files selected by the user (JSON paths):\n[{\"path\":\"relative.txt\"}]", "Attached files selected by the user (JSON paths):\n[{\"path\":\"https://example.com/test\"}]", "Attached files selected by the user (JSON paths):\n[{\"path\":\"/file\",\"unexpected\":true}]"} {
		if len(historyFileReferences(content)) != 0 {
			t.Fatalf("invalid content interpreted as attachment: %s", content)
		}
	}
}

func TestHistoryFileReferencesNeverConsumeTrailingUserText(t *testing.T) {
	const prefix = "Attached files selected by the user (JSON paths):\n"
	for _, suffix := range []string{"\nplease keep this sentence", " {}", " null"} {
		if len(historyFileReferences(prefix+`[{"name":"x","path":"/work/x"}]`+suffix)) != 0 {
			t.Fatalf("trailing text swallowed: %q", suffix)
		}
	}
}
