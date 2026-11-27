package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const twoFieldForm = `{"id":"frm_test","sessionID":"ses_test","title":"Fixture","fields":[{"key":"target","type":"string","title":"Target?","required":true},{"key":"reason","type":"string","title":"Reason?","required":true}]}`
const formGET = "/api/session/ses_test/form"
const singleChoiceForm = `{"id":"frm_test","sessionID":"ses_test","title":"Fixture","fields":[{"key":"q0","type":"string","title":"Platform","description":"Which platform?","options":[{"value":"Android","label":"Android","description":"Phone app"},{"value":"Linux","label":"Linux"}],"custom":true}]}`
const singleChoiceQuestions = `[{"id":"q0","question":"Platform\nWhich platform?","options":[{"label":"Android","description":"Phone app"},{"label":"Linux"}]}]`

func TestLiveOpenCodeRecoversFormWithoutSSE(t *testing.T) {
	binary := os.Getenv("MODEX_TEST_OPENCODE")
	if binary == "" {
		t.Skip("set MODEX_TEST_OPENCODE to the OpenCode v2 executable")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	process, err := StartProcess(ctx, binary, root, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "XDG_CONFIG_HOME=" + filepath.Join(root, "config")})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	client := NewClient(process.BaseURL(), process.Client(), root)
	sess, err := client.CreateSession(ctx, root, nil, "")
	if err != nil || sess.ID == "" {
		t.Fatalf("create fixture session: %v", err)
	}
	synth := NewSynthesizer(root, nil)
	synth.StartTurn(sess.ID, "fixture-turn", root)
	store := NewDecisions(root, nil, synth.TurnID, nil)
	store.ReplyForm = client.ReplyForm
	stream := NewStream(client, synth)
	stream.Forms = store
	for _, tc := range []struct {
		name, form, questions, reply string
	}{
		{"text fields", twoFieldForm, `[{"id":"target","question":"Target?"},{"id":"reason","question":"Reason?"}]`, `{"answers":{"target":{"answers":["android"]},"reason":{"answers":["fixture"]}}}`},
		{"choice option", singleChoiceForm, singleChoiceQuestions, `{"answers":{"q0":{"answers":["Android"]}}}`},
		{"custom answer", singleChoiceForm, singleChoiceQuestions, `{"answers":{"q0":{"answers":["Other platform"]}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var form nativeForm
			if err := json.Unmarshal([]byte(tc.form), &form); err != nil {
				t.Fatal(err)
			}
			var created struct {
				Data nativeForm `json:"data"`
			}
			path := "/api/session/" + sess.ID + "/form"
			body := map[string]any{"title": form.Title, "fields": form.Fields}
			if err := client.request(ctx, http.MethodPost, path, nil, body, &created); err != nil {
				t.Fatal(err)
			}
			// Only run the poll, modelling a native SSE connection that missed creation.
			stream.reconcile(ctx, sess.ID, true)
			cards := store.Pending()
			if len(cards) != 1 || cards[0].ID != decisionKey(sess.ID, created.Data.ID) || cards[0].TurnID != "fixture-turn" {
				t.Fatalf("native pending form did not recover: %v", cards)
			}
			assertJSON(t, cards[0].Context["questions"], tc.questions)
			if err := store.Answer(ctx, cards[0].ID, json.RawMessage(tc.reply)); err != nil {
				t.Fatal(err)
			}
			forms, err := client.listForms(ctx, sess.ID)
			if err != nil || len(forms) != 0 {
				t.Fatalf("native form still pending: %d, %v", len(forms), err)
			}
		})
	}
}

func TestMultipleTextFieldsRequireExactAnswers(t *testing.T) {
	var replies []decisionReply
	store, _ := decisionStore(t, &replies, nil)
	var submitted map[string]string
	store.ReplyForm = func(_ context.Context, sessionID, formID string, answer map[string]string) error {
		if sessionID != "ses_test" || formID != "frm_test" {
			t.Fatalf("reply target %s/%s", sessionID, formID)
		}
		submitted = answer
		return nil
	}
	store.Handle("form.created", json.RawMessage(`{"form":`+twoFieldForm+`}`), "/work")
	cards := store.Pending()
	if len(cards) != 1 {
		t.Fatalf("cards=%v", cards)
	}
	assertJSON(t, cards[0].Context["questions"], `[{"id":"target","question":"Target?"},{"id":"reason","question":"Reason?"}]`)
	for _, invalid := range []string{
		`{"answers":{"target":{"answers":["android"]}}}`,
		`{"answers":{"target":{"answers":["android"]},"extra":{"answers":["why"]}}}`,
		`{"answers":{"target":{"answers":["android"]},"reason":{"answers":[" "]}}}`,
	} {
		if err := store.Answer(context.Background(), cards[0].ID, json.RawMessage(invalid)); err == nil || submitted != nil {
			t.Fatalf("invalid reply reached endpoint: %v %v", submitted, err)
		}
	}
	answer := json.RawMessage(`{"answers":{"target":{"answers":["android"]},"reason":{"answers":[" preserve spacing "]}}}`)
	if err := store.Answer(context.Background(), cards[0].ID, answer); err != nil || !reflect.DeepEqual(submitted, map[string]string{"target": "android", "reason": " preserve spacing "}) {
		t.Fatalf("reply=%v err=%v", submitted, err)
	}
}

func TestNativeSingleChoiceForm(t *testing.T) {
	for _, tc := range []struct {
		name, form, answer string
	}{
		{"option", singleChoiceForm, "Android"},
		{"custom", singleChoiceForm, "Other platform"},
		{"fixed choice", strings.Replace(singleChoiceForm, `"custom":true`, `"custom":false`, 1), ""},
		{"different wire value", strings.Replace(singleChoiceForm, `"value":"Android"`, `"value":"android"`, 1), ""},
		{"multiple selections", strings.Replace(singleChoiceForm, `"type":"string"`, `"type":"array"`, 1), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var replies []decisionReply
			store, _ := decisionStore(t, &replies, nil)
			var submitted map[string]string
			store.ReplyForm = func(_ context.Context, _, _ string, answer map[string]string) error {
				submitted = answer
				return nil
			}
			store.Handle("form.created", json.RawMessage(`{"form":`+tc.form+`}`), "/work")
			cards := store.Pending()
			if tc.answer == "" {
				if len(cards) != 0 {
					t.Fatalf("unsupported form exposed a card: %v", cards)
				}
				return
			}
			if len(cards) != 1 {
				t.Fatalf("cards=%v", cards)
			}
			assertJSON(t, cards[0].Context["questions"], singleChoiceQuestions)
			answer := json.RawMessage(`{"answers":{"q0":{"answers":["` + tc.answer + `"]}}}`)
			if err := store.Answer(context.Background(), cards[0].ID, answer); err != nil || !reflect.DeepEqual(submitted, map[string]string{"q0": tc.answer}) {
				t.Fatalf("reply=%v err=%v", submitted, err)
			}
		})
	}
}

func TestPollReconcilesFormsBeforeHistory(t *testing.T) {
	for _, tc := range []struct {
		name, during, snapshot string
		seed                   bool
		count                  int
	}{
		{"recover missed form", "", `{"data":[` + twoFieldForm + `]}`, false, 1},
		{"empty clears form", "", `{"data":[]}`, true, 0},
		{"missing data preserves form", "", `{}`, true, 1},
		{"null preserves form", "", `{"data":null}`, true, 1},
		{"malformed field preserves form", "", `{"data":[{"id":"frm_malformed","sessionID":"ses_test","fields":[{"key":"answer","type":"string","required":"yes"}]}]}`, true, 1},
		{"null field flag preserves form", "", `{"data":[{"id":"frm_malformed","sessionID":"ses_test","fields":[{"key":"answer","type":"string","required":null}]}]}`, true, 1},
		{"foreign row preserves form", "", `{"data":[{"id":"frm_other","sessionID":"ses_other","fields":[]}]}`, true, 1},
		{"reply wins stale snapshot", "form.replied", `{"data":[` + twoFieldForm + `]}`, false, 0},
		{"cancel wins stale snapshot", "form.cancelled", `{"data":[` + twoFieldForm + `]}`, true, 0},
		{"local reply wins stale snapshot", "local reply", `{"data":[` + twoFieldForm + `]}`, true, 0},
		{"created wins empty snapshot", "form.created", `{"data":[]}`, false, 1},
		{"replacement turn wins", "new turn", `{"data":[` + twoFieldForm + `]}`, false, 0},
		{"HTTP error preserves form", "http error", "", true, 1},
		{"form timeout preserves form", "timeout", "", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collector := &eventCollector{}
			synth := NewSynthesizer("/work", collector.emit)
			store := NewDecisions("/work", collector.emit, synth.TurnID, nil)
			store.ReplyForm = func(context.Context, string, string, map[string]string) error { return nil }
			synth.OnTurnComplete = store.Resolve
			synth.StartTurn("ses_test", "first", "/work")
			created := json.RawMessage(`{"form":` + twoFieldForm + `}`)
			if tc.seed {
				store.Handle("form.created", created, "/work")
			}
			server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ string) {
				if r.URL.Path != formGET {
					http.NotFound(w, r) // Form recovery is independent of history.
					return
				}
				switch tc.during {
				case "form.replied", "form.cancelled":
					store.Handle(tc.during, json.RawMessage(`{"sessionID":"ses_test","id":"frm_test"}`), "/work")
				case "form.created":
					store.Handle(tc.during, created, "/work")
				case "new turn":
					synth.StartTurn("ses_test", "second", "/work")
				case "local reply":
					err := store.Answer(r.Context(), "ses_test/frm_test", json.RawMessage(`{"answers":{"target":{"answers":["platform"]},"reason":{"answers":["test"]}}}`))
					if err != nil {
						t.Errorf("local form reply failed: %v", err)
					}
				case "http error":
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				case "timeout":
					<-r.Context().Done()
					return
				}
				jsonResponse(w, http.StatusOK, tc.snapshot)
			})
			stream := NewStream(NewClient(server.URL, server.Client(), "/work"), synth, store.Handle)
			stream.Forms = store
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stream.reconcile(ctx, "ses_test", true)
			if len(*requests) < 2 || (*requests)[0].Path != formGET || (*requests)[1].Path != "/api/session/ses_test" {
				t.Fatalf("history must follow the form fetch, even on failure: %v", *requests)
			}
			if got := store.Pending(); len(got) != tc.count {
				t.Fatalf("pending=%v want count %d; events=%v", got, tc.count, eventMethods(collector.snapshot()))
			}
			if _, active := synth.ActiveTurn("ses_test"); !active {
				t.Fatal("form reconciliation ended the turn")
			}
		})
	}
}
