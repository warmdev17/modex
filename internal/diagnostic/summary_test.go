package diagnostic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSummaryDoesNotExposeProviderSecrets(t *testing.T) {
	raw := []byte(`{"config":{"model_provider":"third-party","model":"test-model","model_providers":{"third-party":{"base_url":"https://secret.example/token","experimental_bearer_token":"secret-key","wire_api":"responses"}}}}`)
	got, err := Summarize(raw)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), "secret") {
		t.Fatalf("secret exposed: %s", b)
	}
	if got.Provider != "third-party" || got.Model != "test-model" {
		t.Fatalf("wrong summary: %+v", got)
	}
}

func TestSummaryRejectsInvalidResponse(t *testing.T) {
	for _, raw := range []string{`{`, `{}`, `{"config":null}`} {
		if _, err := Summarize([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
