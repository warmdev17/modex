package bridge

// Keep Desktop-owned project identity and roots without exposing arbitrary metadata.
type projectEntry struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Roots []struct {
		Path string `json:"path"`
	} `json:"roots"`
}
