package antigravity

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/warmdev/modex/internal/acp"
)

type attachment struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
	Kind string `json:"kind"`
}

func attachmentPath(path string) string {
	if runtime.GOOS == "windows" && len(path) > 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}
func validAttachmentPath(path string) bool {
	return filepath.IsAbs(path) && len(path) <= 32768 && !strings.ContainsFunc(path, unicode.IsControl)
}
func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}
func replayAttachment(uri, mime string) *attachment {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Host != "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.HasPrefix(u.Path, "//") {
		return nil
	}
	path := attachmentPath(u.Path)
	if !validAttachmentPath(path) {
		return nil
	}
	kind := "file"
	if mime == "image/png" || mime == "image/jpeg" {
		kind = "image"
	}
	metadataPath := path
	if runtime.GOOS == "windows" {
		metadataPath = u.Path
	}
	return &attachment{Path: metadataPath, Name: filepath.Base(path), Kind: kind}
}
func prepareAttachments(raw json.RawMessage, info acp.Info) ([]acp.PromptBlock, []attachment, error) {
	if raw == nil {
		return nil, nil, nil
	}
	var input []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || len(input) == 0 || len(input) > 8 {
		return nil, nil, errors.New("select up to 8 attachments")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, nil, errors.New("invalid attachments")
	}
	var blocks []acp.PromptBlock
	var metadata []attachment
	total := 0
	for _, a := range input {
		path := attachmentPath(a.Path)
		if !validAttachmentPath(path) || (a.Kind != "file" && a.Kind != "image") {
			return nil, nil, errors.New("invalid attachment path or kind")
		}
		if a.Kind == "image" && !info.Image {
			return nil, nil, errors.New("Antigravity does not support image attachments")
		}
		if a.Kind == "file" && !info.EmbeddedContext {
			return nil, nil, errors.New("Antigravity does not support text attachments")
		}
		stat, err := os.Stat(path)
		if err != nil || !stat.Mode().IsRegular() {
			return nil, nil, errors.New("attachment is unavailable or not a regular file")
		}
		f, err := openAttachment(path)
		if err != nil {
			return nil, nil, errors.New("attachment is not readable")
		}
		stat, err = f.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			f.Close()
			return nil, nil, errors.New("attachment is unavailable or not a regular file")
		}
		limit := 8*1024*1024 - total
		if a.Kind == "file" && limit > 256*1024 {
			limit = 256 * 1024
		}
		data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return nil, nil, errors.New("attachment is not readable")
		}
		if len(data) == 0 || len(data) > limit {
			return nil, nil, errors.New("attachment is empty or exceeds the size limit")
		}
		total += len(data)
		uri := fileURI(path)
		if a.Kind == "image" {
			mime := http.DetectContentType(data)
			if mime != "image/png" && mime != "image/jpeg" {
				return nil, nil, errors.New("select a PNG or JPEG image")
			}
			blocks = append(blocks, acp.PromptBlock{Type: "image", Data: base64.StdEncoding.EncodeToString(data), MimeType: mime, URI: uri})
		} else {
			if !utf8.Valid(data) || bytes.IndexFunc(data, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' && r != '\f' }) >= 0 {
				return nil, nil, errors.New("select a UTF-8 text file")
			}
			blocks = append(blocks, acp.PromptBlock{Type: "resource", Resource: &acp.PromptResource{URI: uri, MimeType: "text/plain", Text: string(data)}})
		}
		metadata = append(metadata, attachment{Path: a.Path, Name: filepath.Base(path), Kind: a.Kind})
	}
	return blocks, metadata, nil
}
