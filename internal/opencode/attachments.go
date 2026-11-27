package opencode

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

type chatAttachment struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type nativeAttachment struct {
	Mime   string `json:"mime"`
	Source struct {
		Type string `json:"type"`
		URI  string `json:"uri"`
	} `json:"source"`
}

func validAttachmentPath(path string) bool {
	return filepath.IsAbs(path) && len(path) <= 32768 && !strings.ContainsFunc(path, unicode.IsControl)
}

func attachmentPath(path string) string {
	if runtime.GOOS == "windows" && len(path) > 3 && path[0] == '/' && path[2] == ':' {
		return path[1:]
	}
	return path
}

func projectAttachments(files []nativeAttachment) []chatAttachment {
	var attachments []chatAttachment
	for _, file := range files {
		if len(attachments) == 32 {
			break
		}
		if file.Source.Type != "uri" {
			continue
		}
		uri, err := url.Parse(file.Source.URI)
		if err != nil || uri.Scheme != "file" || uri.Host != "" || uri.User != nil || uri.Opaque != "" ||
			uri.RawQuery != "" || uri.ForceQuery || uri.Fragment != "" || strings.HasPrefix(uri.Path, "//") {
			continue
		}
		path := filepath.FromSlash(attachmentPath(uri.Path))
		if !validAttachmentPath(path) {
			continue
		}
		kind := "file"
		switch file.Mime {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
			kind = "image"
		}
		attachments = append(attachments, chatAttachment{Path: path, Name: filepath.Base(path), Kind: kind})
	}
	return attachments
}
