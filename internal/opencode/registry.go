package opencode

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const maxRegistrySize = 4 << 20

type registeredChat struct {
	ID          string `json:"id"`
	Native      string `json:"native_id"`
	Workspace   string `json:"workspace"`
	Project     string `json:"project_id,omitempty"`
	Projectless bool   `json:"projectless,omitempty"`
}

type registry struct {
	mu    sync.Mutex
	path  string
	chats map[string]registeredChat
}

func openRegistry(dir string) (*registry, error) {
	if err := validatePrivateDirectory(dir); err != nil {
		return nil, errors.New("invalid private opencode directory")
	}
	r := &registry{path: filepath.Join(dir, "opencode-threads.json"), chats: map[string]registeredChat{}}
	if _, err := os.Lstat(r.path); os.IsNotExist(err) {
		return r, nil
	} else if err != nil || validatePrivateFile(r.path) != nil {
		return nil, errors.New("invalid opencode session state")
	}
	f, err := os.Open(r.path)
	if err != nil {
		return nil, errors.New("cannot read opencode session state")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxRegistrySize+1))
	if err != nil || len(raw) > maxRegistrySize {
		return nil, errors.New("invalid opencode session state")
	}
	var records []registeredChat
	if json.Unmarshal(raw, &records) != nil || records == nil {
		return nil, errors.New("invalid opencode session state")
	}
	seen := map[string]bool{}
	for _, v := range records {
		if !strings.HasPrefix(v.ID, "oc:") || !validRegistryText(v.ID) || !validRegistryText(v.Native) || !filepath.IsAbs(v.Workspace) || r.chats[v.ID].ID != "" || seen[v.Workspace+"\x00"+v.Native] {
			return nil, errors.New("invalid opencode session state")
		}
		seen[v.Workspace+"\x00"+v.Native] = true
		r.chats[v.ID] = v
	}
	return r, nil
}

func validRegistryText(s string) bool {
	return strings.TrimSpace(s) != "" && len(s) <= 4096 && !strings.ContainsAny(s, "\r\n\x00")
}

func (r *registry) add(native, workspace, project string, projectless bool) (registeredChat, error) {
	if !validRegistryText(native) || !filepath.IsAbs(workspace) || (project != "" && !validRegistryText(project)) {
		return registeredChat{}, errors.New("invalid opencode session")
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return registeredChat{}, errors.New("cannot create opencode identity")
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	v := registeredChat{ID: fmt.Sprintf("oc:%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), Native: native, Workspace: workspace, Project: project, Projectless: projectless}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, old := range r.chats {
		if old.Workspace == workspace && old.Native == native {
			return registeredChat{}, errors.New("duplicate opencode session")
		}
	}
	r.chats[v.ID] = v
	if err := r.save(); err != nil {
		delete(r.chats, v.ID)
		return registeredChat{}, err
	}
	return v, nil
}

func (r *registry) save() error {
	records := make([]registeredChat, 0, len(r.chats))
	for _, v := range r.chats {
		records = append(records, v)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	data, err := json.Marshal(records)
	if err != nil || len(data) > maxRegistrySize {
		return errors.New("opencode session state exceeds limit")
	}
	f, err := os.CreateTemp(filepath.Dir(r.path), ".opencode-threads-*")
	if err != nil {
		return errors.New("cannot save opencode session state")
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = os.Rename(name, r.path)
	}
	if err != nil {
		return errors.New("cannot save opencode session state")
	}
	return nil
}

func (r *registry) get(id string) (registeredChat, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.chats[id]
	return v, ok
}
