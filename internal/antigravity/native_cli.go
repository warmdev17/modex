package antigravity

import (
	"context"
	"errors"
	"os/exec"
)

// NativeCLI resolves a registered conversation for the authenticated local CLI.
func (r *Router) NativeCLI(ctx context.Context, workspace, threadID string) (map[string]string, error) {
	path, err := canonical(workspace)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	binary, err := exec.LookPath(r.binary)
	if err != nil {
		return nil, errors.New("Install the agy CLI on the host")
	}
	r.mu.Lock()
	c := r.chats[threadID]
	if c == nil || c.Workspace != path {
		r.mu.Unlock()
		return nil, errors.New("Antigravity session outside workspace")
	}
	if c.active != "" || c.loading || c.setting || c.nativeTerminal {
		r.mu.Unlock()
		return nil, errors.New("Antigravity session is busy")
	}
	native, model, thinking := c.Native, c.Model, c.Thinking
	c.nativeTerminal = true
	r.mu.Unlock()
	return map[string]string{"binary": binary, "workspace": path, "session_id": native, "model": model, "thinking": thinking}, nil
}

// ReleaseNativeCLI invalidates cached history after the native terminal exits.
func (r *Router) ReleaseNativeCLI(workspace, threadID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c := r.chats[threadID]; c != nil && c.Workspace == workspace && c.active == "" {
		c.loaded = false
		c.nativeTerminal = false
	}
}

// ReleaseNativeCLIs clears reservations when the owning local CLI disconnects.
func (r *Router) ReleaseNativeCLIs() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.chats {
		if c.nativeTerminal {
			c.nativeTerminal = false
			c.loaded = false
		}
	}
}
