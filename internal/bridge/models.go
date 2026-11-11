package bridge

// Project only fields needed by the phone picker.
type modelEntry struct {
	ID            string `json:"id"`
	Model         string `json:"model"`
	DisplayName   string `json:"displayName"`
	DefaultEffort string `json:"defaultReasoningEffort"`
	Efforts       []struct {
		Effort      string `json:"reasoningEffort"`
		Description string `json:"description"`
	} `json:"supportedReasoningEfforts"`
}
