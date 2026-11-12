package bridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const maxAttachmentBytes = 20 * 1024 * 1024

type attachmentInput struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

func turnInput(text string, raw json.RawMessage) ([]map[string]string, error) {
	input := []map[string]string{}
	if strings.TrimSpace(text) != "" {
		input = append(input, map[string]string{"type": "text", "text": text})
	}
	if raw == nil {
		if len(input) == 0 {
			return nil, errors.New("text or attachments are required")
		}
		return input, nil
	}
	var attachments []attachmentInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&attachments) != nil || len(attachments) < 1 || len(attachments) > 8 {
		return nil, errors.New("invalid attachments; select up to 8 files")
	}
	files := []map[string]string{}
	for _, a := range attachments {
		// Windows SFTP paths may expose drive letters beneath a synthetic slash root.
		if runtime.GOOS == "windows" && len(a.Path) > 3 && a.Path[0] == '/' && a.Path[2] == ':' {
			a.Path = a.Path[1:]
		}
		if !filepath.IsAbs(a.Path) || len(a.Path) > 32768 || strings.ContainsAny(a.Path, "\r\n\x00") || (a.Kind != "image" && a.Kind != "file") {
			return nil, errors.New("invalid attachment path or kind")
		}
		info, err := os.Stat(a.Path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxAttachmentBytes {
			return nil, errors.New("attachment unavailable, not a regular file, or over 20 MiB")
		}
		file, err := os.Open(a.Path)
		if err != nil {
			return nil, errors.New("attachment is not readable")
		}
		var head [512]byte
		n, _ := file.Read(head[:])
		_ = file.Close()
		if a.Kind == "image" {
			mime := http.DetectContentType(head[:n])
			if mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" && mime != "image/webp" {
				return nil, errors.New("select PNG, JPEG, GIF or WebP images")
			}
			input = append(input, map[string]string{"type": "localImage", "path": a.Path})
		} else {
			files = append(files, map[string]string{"name": filepath.Base(a.Path), "path": a.Path})
		}
	}
	if len(files) > 0 {
		encoded, _ := json.Marshal(files)
		input = append(input, map[string]string{"type": "text", "text": "Attached files selected by the user (JSON paths):\n" + string(encoded)})
	}
	return input, nil
}

// Only the exact Modex file-reference format becomes preview metadata; never fetch URLs.
func historyFileReferences(text string) []map[string]string {
	const prefix = "Attached files selected by the user (JSON paths):\n"
	if !strings.HasPrefix(text, prefix) {
		return nil
	}
	var files []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	decoder := json.NewDecoder(strings.NewReader(strings.TrimPrefix(text, prefix)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&files) != nil || len(files) < 1 || len(files) > 8 {
		return nil
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil
	}
	result := []map[string]string{}
	for _, file := range files {
		if !filepath.IsAbs(file.Path) || strings.ContainsAny(file.Path, "\r\n\x00") || len(file.Path) > 32768 {
			return nil
		}
		result = append(result, map[string]string{"path": file.Path, "name": filepath.Base(file.Path), "kind": "file"})
	}
	return result
}
