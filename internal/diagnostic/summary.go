package diagnostic

import (
	"encoding/json"
	"errors"
)

// Summary deliberately excludes credentials, endpoints and raw configuration.
type Summary struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func Summarize(raw []byte) (Summary, error) {
	var response struct {
		Config *struct {
			Provider string `json:"model_provider"`
			Model    string `json:"model"`
		} `json:"config"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return Summary{}, errors.New("invalid config response")
	}
	if response.Config == nil {
		return Summary{}, errors.New("config missing from response")
	}
	provider := response.Config.Provider
	if provider == "" {
		provider = "openai"
	}
	return Summary{Provider: provider, Model: response.Config.Model}, nil
}
