package opencode

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
}

func fakeServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, body string)) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	requests := []recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, recordedRequest{r.Method, r.URL.Path, r.URL.RawQuery, string(body)})
		mu.Unlock()
		handle(w, r, string(body))
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func jsonResponse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func TestClientExactRequests(t *testing.T) {
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		switch r.URL.Path {
		case "/api/info":
			jsonResponse(w, http.StatusOK, `{"version":"2.4.1","pid":1,"urls":[]}`)
		case "/api/config":
			jsonResponse(w, http.StatusOK, `[{"type":"document","path":"/cfg","info":{"default_agent":"builder","agents":{"builder":{"model":{"providerID":"p1","model":"m1"}}}}}]`)
		case "/api/session":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"ses_1","location":{"directory":"/work"},"model":{"id":"m1","providerID":"p1"}}}`)
		case "/api/session/ses_1":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"ses_1","location":{"directory":"/work"}}}`)
		case "/api/session/ses_1/message":
			jsonResponse(w, http.StatusOK, `{"data":[],"cursor":{"previous":null,"next":null}}`)
		case "/api/session/ses_1/interrupt":
			jsonResponse(w, http.StatusOK, `{"interrupted":false}`)
		case "/api/model":
			jsonResponse(w, http.StatusOK, `{"location":{"directory":"/work"},"data":[{"id":"m1","modelID":"m1","providerID":"p1","name":"M1"}]}`)
		default:
			http.NotFound(w, r)
		}
	})
	client := NewClient(server.URL, server.Client(), "/work")
	ctx := context.Background()

	if _, err := client.Info(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Config(ctx); err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateSession(ctx, "/work", &modelRef{ProviderID: "p1", ModelID: "m1"}, "builder")
	if err != nil || created.ID != "ses_1" {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	if _, err := client.ReadSession(ctx, "ses_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListMessagesPage(ctx, "ses_1", 10, "asc", "abc"); err != nil {
		t.Fatal(err)
	}
	if err := client.Abort(ctx, "ses_1"); err != nil {
		t.Fatal(err)
	}
	if models, err := client.ListModels(ctx); err != nil || len(models) != 1 || models[0] != (modelRef{"p1", "m1"}) {
		t.Fatalf("models=%+v err=%v", models, err)
	}

	want := []recordedRequest{
		{"GET", "/api/info", "", ""},
		{"GET", "/api/config", "directory=%2Fwork", ""},
		{"POST", "/api/session", "", `{"agent":"builder","location":{"directory":"/work"},"model":{"id":"m1","providerID":"p1"}}`},
		{"GET", "/api/session/ses_1", "", ""},
		{"GET", "/api/session/ses_1/message", "cursor=abc&limit=10&order=asc", ""},
		{"POST", "/api/session/ses_1/interrupt", "", ""},
		{"GET", "/api/model", "directory=%2Fwork", ""},
	}
	if !reflect.DeepEqual(*requests, want) {
		t.Fatalf("requests=%+v\nwant=%+v", *requests, want)
	}
}

func TestClientListSessionsFollowsCursorAndSlices(t *testing.T) {
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		switch r.URL.Query().Get("cursor") {
		case "":
			jsonResponse(w, http.StatusOK, `{"data":[{"id":"ses_1","location":{"directory":"/work"}}],"cursor":{"previous":null,"next":"c2"}}`)
		case "c2":
			jsonResponse(w, http.StatusOK, `{"data":[{"id":"ses_2","location":{"directory":"/work"}}],"cursor":{"previous":null,"next":null}}`)
		default:
			http.NotFound(w, r)
		}
	})
	client := NewClient(server.URL, server.Client(), "/work")
	sessions, err := client.ListSessions(context.Background(), 2)
	if err != nil || len(sessions) != 2 || sessions[0].ID != "ses_1" || sessions[1].ID != "ses_2" {
		t.Fatalf("sessions=%+v err=%v", sessions, err)
	}
	want := []recordedRequest{
		{"GET", "/api/session", "limit=100&order=desc", ""},
		{"GET", "/api/session", "cursor=c2&limit=100&order=desc", ""},
	}
	if !reflect.DeepEqual(*requests, want) {
		t.Fatalf("requests=%+v\nwant=%+v", *requests, want)
	}
}

func TestClientListModelsRetriesColdCatalog(t *testing.T) {
	calls := 0
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if r.URL.Path != "/api/model" {
			http.NotFound(w, r)
			return
		}
		calls++
		if calls == 1 {
			jsonResponse(w, http.StatusOK, `{"location":{"directory":"/work"},"data":[]}`)
			return
		}
		jsonResponse(w, http.StatusOK, `{"location":{"directory":"/work"},"data":[{"id":"m1","modelID":"m1","providerID":"p1","name":"M1"}]}`)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	models, err := client.ListModels(context.Background())
	if err != nil || len(models) != 1 || models[0] != (modelRef{"p1", "m1"}) {
		t.Fatalf("models=%+v err=%v", models, err)
	}
	if len(*requests) != 2 {
		t.Fatalf("requests=%+v", *requests)
	}
}

func TestClientSanitizesUpstreamErrors(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		jsonResponse(w, http.StatusInternalServerError, `{"error":"SECRET-TOKEN and prompt text"}`)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	_, err := client.Info(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "SECRET-TOKEN") || strings.Contains(err.Error(), "prompt text") {
		t.Fatalf("error leaked the upstream body: %v", err)
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("error=%v", err)
	}
}

func TestClientRejectsInvalidJSON(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		jsonResponse(w, http.StatusOK, `not json`)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	if _, err := client.Info(context.Background()); err == nil {
		t.Fatal("expected an invalid response error")
	}
}

func TestResolveDefaultModel(t *testing.T) {
	docs := []configDocument{
		{Type: "document", Info: configInfo{DefaultAgent: "planner", Agents: map[string]configAgent{
			"planner": {Model: json.RawMessage(`{"providerID":"p1","model":"m1"}`)},
		}}},
		{Type: "document", Info: configInfo{DefaultAgent: "builder", Agents: map[string]configAgent{
			"builder": {Model: json.RawMessage(`"p2/m2#high"`)},
		}}},
	}
	provider, model := resolveDefaultModel(docs)
	if provider != "p2" || model != "m2" {
		t.Fatalf("provider=%q model=%q", provider, model)
	}
	provider, model = resolveDefaultModel([]configDocument{{Type: "directory", Path: "/x"}})
	if provider != "" || model != "" {
		t.Fatalf("fallback provider=%q model=%q", provider, model)
	}
}
