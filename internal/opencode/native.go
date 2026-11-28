package opencode

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
)

// NativeCLI returns credentials only to the host's authenticated local CLI
// handoff. Its result must never be journaled or sent to a remote client.
func (r *Router) NativeCLI(ctx context.Context, workspace, threadID string) (map[string]string, error) {
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	if r.registry == nil {
		return nil, errors.New("OpenCode session not found")
	}
	chat, ok := r.registry.get(threadID)
	if !ok {
		return nil, errors.New("OpenCode session not found")
	}
	path, err := canonicalWorkspace(workspace)
	if err != nil || path != chat.Workspace {
		return nil, errors.New("thread outside workspace")
	}
	binary, err := exec.LookPath(r.binary)
	if err != nil {
		return nil, errors.New("OpenCode executable is unavailable")
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return nil, errors.New("OpenCode executable is unavailable")
	}
	w, err := r.workspace(path)
	if err != nil {
		return nil, err
	}
	w.stream.nativeMu.Lock()
	defer w.stream.nativeMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	session, err := w.client.ReadSession(ctx, chat.Native)
	if err != nil {
		return nil, err
	}
	if session.ID != chat.Native || session.Location.Directory != path {
		return nil, errors.New("thread outside workspace")
	}
	if _, active := w.synth.ActiveTurn(chat.Native); active || session.Time.Busy != nil && *session.Time.Busy > 0 && (session.Time.Idle == nil || *session.Time.Busy > *session.Time.Idle) {
		return nil, errors.New("OpenCode session is busy")
	}
	if w.stream.native == nil {
		w.stream.native = map[string]*nativeWatch{}
	}
	watch := w.stream.native[chat.Native]
	if watch == nil {
		if len(w.stream.native) >= 16 {
			return nil, errors.New("too many native OpenCode sessions")
		}
		page, err := w.client.ListMessagesPage(ctx, chat.Native, 1, "desc", "")
		if err != nil {
			return nil, err
		}
		if w.decisions != nil {
			permissions, err := w.client.ListPermissions(ctx, chat.Native)
			if err != nil {
				return nil, errors.New("cannot verify pending OpenCode requests")
			}
			forms, err := w.client.listForms(ctx, chat.Native)
			if err != nil {
				return nil, errors.New("cannot verify pending OpenCode requests")
			}
			if err := w.decisions.excludeExisting(chat.Native, permissions, forms); err != nil {
				return nil, err
			}
		}
		watch = &nativeWatch{}
		if len(page.Data) > 0 {
			watch.anchor = page.Data[0].ID
			if watch.anchor == "" {
				return nil, errors.New("invalid OpenCode history")
			}
		}
		w.stream.native[chat.Native] = watch
	}
	watch.refs++
	r.mu.Lock()
	p := r.process
	r.mu.Unlock()
	return map[string]string{"binary": binary, "workspace": path, "session_id": chat.Native, "server": p.baseURL, "password": p.password}, nil
}

// ReleaseNativeCLI stops adopting new native turns once the last TUI exits.
// An accepted running turn still uses the normal active-turn reconciler.
func (r *Router) ReleaseNativeCLI(threadID string) {
	if r.registry == nil {
		return
	}
	chat, ok := r.registry.get(threadID)
	if !ok {
		return
	}
	r.mu.Lock()
	w := r.workspaces[chat.Workspace]
	r.mu.Unlock()
	if w == nil || w.stream == nil {
		return
	}
	w.stream.nativeMu.Lock()
	defer w.stream.nativeMu.Unlock()
	if watch := w.stream.native[chat.Native]; watch != nil {
		watch.refs--
		if watch.refs <= 0 {
			delete(w.stream.native, chat.Native)
		}
	}
}

// ReleaseNativeCLIs drops watches when the authenticated local CLI lease ends.
// Accepted turns remain owned by the normal active-turn reconciler.
func (r *Router) ReleaseNativeCLIs() {
	r.mu.Lock()
	workspaces := make([]*managedWorkspace, 0, len(r.workspaces))
	for _, w := range r.workspaces {
		workspaces = append(workspaces, w)
	}
	r.mu.Unlock()
	for _, w := range workspaces {
		if w.stream == nil {
			continue
		}
		w.stream.nativeMu.Lock()
		w.stream.native = nil
		w.stream.nativeMu.Unlock()
	}
}

type nativeWatch struct {
	anchor string
	refs   int
}

// reconcileNative adopts only messages added after an explicit local handoff.
// Keeping the native mutex across submission and polling prevents adopting a
// Modex prompt before its normal acknowledgement has established its turn.
func (s *Stream) reconcileNative(ctx context.Context, sessionID string) {
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	_ = s.reconcileNativeLocked(ctx, sessionID)
}

func (s *Stream) reconcileNativeLocked(ctx context.Context, sessionID string) error {
	watch := s.native[sessionID]
	if watch == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	session, err := s.client.ReadSession(ctx, sessionID)
	if err != nil || session.ID != sessionID || session.Location.Directory != s.client.cwd {
		return errors.New("cannot verify pending native OpenCode messages")
	}
	messages, ok := s.nativeMessages(ctx, sessionID, watch.anchor)
	if !ok {
		return errors.New("cannot verify pending native OpenCode messages")
	}
	first := 0
	if watch.anchor != "" {
		for i, m := range messages {
			if m.ID == watch.anchor {
				first = i + 1
				break
			}
		}
	}
	for i := first; i < len(messages); i++ {
		m := messages[i]
		if m.Type != "user" || m.ID == "" {
			continue
		}
		s.synth.Reconcile(session, messages[:i])
		turnID, active := s.synth.ActiveTurn(sessionID)
		adopted := false
		if !active && turnID != m.ID && m.Metadata.TurnID == "" {
			s.synth.StartTurn(sessionID, m.ID, session.Location.Directory)
			turnID, active, adopted = m.ID, true, true
		}
		// Modex already emits its confirmed user item. Steers carry its turn id.
		if active && m.Metadata.TurnID == "" && (adopted || turnID != m.ID) {
			content := []map[string]string{{"type": "text", "text": messageText(m)}}
			item := map[string]any{"id": m.ID, "type": "userMessage", "content": content}
			if attachments := projectAttachments(m.Files); len(attachments) > 0 {
				item["attachments"] = attachments
			}
			s.synth.emitEvent("item/completed", map[string]any{"threadId": sessionID, "turnId": turnID, "item": item})
		}
	}
	s.synth.Reconcile(session, messages)
	if len(messages) > 0 {
		watch.anchor = messages[len(messages)-1].ID
	}
	if _, active := s.synth.ActiveTurn(sessionID); active {
		s.reconcile(ctx, sessionID, true)
	}
	return nil
}

func (s *Stream) nativeMessages(ctx context.Context, sessionID, anchor string) ([]message, bool) {
	var messages []message
	cursor := ""
	seen := map[string]bool{}
	// shortcut: a native watch recovers at most 100 messages per poll; widen
	// this window before supporting larger disconnected TUI bursts.
	for len(messages) < 100 {
		page, err := s.client.ListMessagesPage(ctx, sessionID, min(pollMessages, 100-len(messages)), "desc", cursor)
		if err != nil {
			return nil, false
		}
		messages = append(messages, page.Data...)
		found := false
		for _, m := range page.Data {
			if m.ID == anchor && anchor != "" {
				found = true
				break
			}
		}
		exhausted := len(page.Data) == 0 || page.Cursor.Next == nil || *page.Cursor.Next == ""
		if found || exhausted {
			if !found && anchor != "" {
				return nil, false
			}
			reverseMessages(messages)
			return messages, true
		}
		cursor = *page.Cursor.Next
		if seen[cursor] {
			return nil, false
		}
		seen[cursor] = true
	}
	return nil, false
}
