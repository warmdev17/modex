package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLargeAttachmentConfirmationAndHistory(t *testing.T) {
	encoded := strings.Repeat("AAAA", (20<<20)/3)
	var promptCalls atomic.Int32
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		var prefix, suffix string
		switch r.URL.Path {
		case "/api/session/s/prompt":
			promptCalls.Add(1)
			prefix, suffix = `{"data":{"payload":{"files":[{"data":"`, `"}]},"id":"u","type":"user"}}`
		case "/api/session/s/message":
			prefix, suffix = `{"data":[{"id":"u","type":"user","text":"literal data","files":[{"data":"`, `","mime":"text/plain","source":{"type":"uri","uri":"file:///tmp/test.txt"}}]}],"cursor":{"next":"next"}}`
		default:
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, prefix)
		_, _ = io.WriteString(w, encoded)
		_, _ = io.WriteString(w, suffix)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	id, err := client.Prompt(context.Background(), "s", "", []promptFile{{URI: "file:///tmp/test.txt"}}, nil, "")
	if err != nil || id != "u" {
		t.Errorf("prompt id=%q err=%v", id, err)
	}
	page, err := client.ListMessagesPage(context.Background(), "s", 10, "desc", "")
	if err != nil || len(page.Data) != 1 || page.Data[0].Text != "literal data" ||
		len(page.Data[0].Files) != 1 || page.Data[0].Files[0].Source.URI != "file:///tmp/test.txt" ||
		page.Cursor.Next == nil || *page.Cursor.Next != "next" || promptCalls.Load() != 1 {
		t.Fatalf("history=%+v err=%v prompts=%d", page, err, promptCalls.Load())
	}
}

func TestAttachmentResponsePreservesOtherDataAndRejectsTruncation(t *testing.T) {
	const body = `{"data":[{"files":[{"data":"////","name":"q\"uote","source":{"data":"keep"}}],"metadata":{"files":[{"data":"keep too"}]}}]}`
	out, err := readAttachmentResponse(strings.NewReader(body), historyAttachmentPath, int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, json.RawMessage(out), `{"data":[{"files":[{"data":"","name":"q\"uote","source":{"data":"keep"}}],"metadata":{"files":[{"data":"keep too"}]}}]}`)
	if _, err := readAttachmentResponse(strings.NewReader(body), historyAttachmentPath, 34); !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("limited response error=%v", err)
	}
	if _, err := readAttachmentResponse(strings.NewReader(`{"data":[{"files":[{"data":"abcd`), historyAttachmentPath, 500); !errors.Is(err, errInvalidResponse) {
		t.Fatalf("truncated attachment error=%v", err)
	}
	const object = `{"data":{"payload":{"files":{"[]":{"data":"keep"}}}}}`
	out, err = readAttachmentResponse(strings.NewReader(object), promptAttachmentPath, 500)
	if err != nil || string(out) != object {
		t.Fatalf("object key mistaken for array entry: %s err=%v", out, err)
	}
}

func TestHistoryShrinksOversizedPageWithoutDroppingCursor(t *testing.T) {
	var requests []string
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		requests = append(requests, r.URL.RawQuery)
		if r.URL.Query().Get("limit") == "6" {
			w.Header().Set("Content-Length", "268435457")
			w.WriteHeader(http.StatusOK)
			return
		}
		jsonResponse(w, http.StatusOK, `{"data":[{"id":"u","type":"user","text":"hello"}],"cursor":{"next":"after"}}`)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	page, err := client.ListMessagesPage(context.Background(), "s", 6, "desc", "before")
	if err != nil || page.Cursor.Next == nil || *page.Cursor.Next != "after" ||
		strings.Join(requests, ",") != "cursor=before&limit=6,cursor=before&limit=3" {
		t.Fatalf("page=%+v err=%v requests=%v", page, err, requests)
	}
}
