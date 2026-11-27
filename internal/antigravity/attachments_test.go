package antigravity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warmdev/modex/internal/acp"
)

func TestAttachmentValidationAndNativeShapes(t *testing.T) {
	root := t.TempDir()
	text := filepath.Join(root, "note #1.txt")
	image := filepath.Join(root, "image.png")
	bad := filepath.Join(root, "bad.txt")
	binary := filepath.Join(root, "binary.txt")
	large := filepath.Join(root, "large.txt")
	empty := filepath.Join(root, "empty.txt")
	for path, data := range map[string][]byte{text: []byte("note"), image: []byte("\x89PNG\r\n\x1a\nexample"), bad: {0xff}, binary: {1, 2}, large: []byte(strings.Repeat("x", 256*1024+1)), empty: {}} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw := func(path, kind string) json.RawMessage {
		b, _ := json.Marshal([]map[string]string{{"path": path, "kind": kind}})
		return b
	}
	capabilities := acp.Info{Image: true, EmbeddedContext: true}
	for _, tc := range []struct {
		name       string
		path, kind string
		info       acp.Info
	}{
		{"unsupported image", image, "image", acp.Info{}}, {"unsupported text", text, "file", acp.Info{}}, {"wrong MIME", text, "image", capabilities}, {"invalid UTF8", bad, "file", capabilities}, {"binary controls", binary, "file", capabilities}, {"large text", large, "file", capabilities}, {"empty", empty, "file", capabilities}, {"directory", root, "file", capabilities}, {"relative", "relative", "file", capabilities},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := prepareAttachments(raw(tc.path, tc.kind), tc.info); err == nil || strings.Contains(err.Error(), tc.path) {
				t.Fatalf("expected private rejection, got %v", err)
			}
		})
	}
	blocks, meta, err := prepareAttachments(raw(image, "image"), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(blocks)
	if !strings.Contains(string(b), `"type":"image"`) || !strings.Contains(string(b), `"mimeType":"image/png"`) || len(meta) != 1 || meta[0].Name != "image.png" {
		t.Fatalf("blocks=%s metadata=%v", b, meta)
	}
}
func TestReplayOnlyLocalFileURIs(t *testing.T) {
	for _, uri := range []string{"https://example.com/a", "file://remote/tmp/a", "file:///tmp/a?x=1", "file:///tmp/a%00b", "file:relative"} {
		if a := replayAttachment(uri, "text/plain"); a != nil {
			t.Fatalf("accepted %q", uri)
		}
	}
	a := replayAttachment("file:///tmp/note%20%231.txt", "text/plain")
	if a == nil || a.Path != "/tmp/note #1.txt" || a.Kind != "file" {
		t.Fatalf("projection: %+v", a)
	}
}

func TestAttachmentCountAndAggregateSizeLimits(t *testing.T) {
	root := t.TempDir()
	image := func(name string, size int) string {
		t.Helper()
		path := filepath.Join(root, name)
		data := make([]byte, size)
		copy(data, "\x89PNG\r\n\x1a\n")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	first := image("first.png", 4*1024*1024)
	exact := image("exact.png", 4*1024*1024)
	over := image("over.png", 4*1024*1024+1)
	small := image("small.png", 8)
	encode := func(paths ...string) json.RawMessage {
		entries := make([]map[string]string, 0, len(paths))
		for _, path := range paths {
			entries = append(entries, map[string]string{"path": path, "kind": "image"})
		}
		raw, err := json.Marshal(entries)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	for _, tc := range []struct {
		name      string
		paths     []string
		wantError bool
	}{
		{"exact 8 MiB", []string{first, exact}, false},
		{"over 8 MiB", []string{first, over}, true},
		{"eight files", []string{small, small, small, small, small, small, small, small}, false},
		{"nine files", []string{small, small, small, small, small, small, small, small, small}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks, metadata, err := prepareAttachments(encode(tc.paths...), acp.Info{Image: true})
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v", err)
			}
			if err == nil && (len(blocks) != len(tc.paths) || len(metadata) != len(tc.paths)) {
				t.Fatalf("blocks=%d metadata=%d", len(blocks), len(metadata))
			}
		})
	}
}
