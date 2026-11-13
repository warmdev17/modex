package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/warmdev/modex/internal/hoststate"
)

type supervisorEndpoint struct {
	Address   string `json:"address"`
	Token     string `json:"token"`
	Instance  string `json:"instance"`
	Workspace string `json:"workspace"`
}

func claimSupervisorState(dir string) (func(), error) {
	release, err := hoststate.Acquire(dir)
	if err != nil {
		return nil, err
	}
	endpoint := filepath.Join(dir, "endpoint.json")
	if _, err := os.Lstat(endpoint); err == nil {
		if err := checkPrivatePath(endpoint, false); err != nil {
			release()
			return nil, err
		}
		if err := os.Remove(endpoint); err != nil {
			release()
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		release()
		return nil, err
	}
	return func() { _ = os.Remove(endpoint); release() }, nil
}

func writeEndpoint(dir string, endpoint supervisorEndpoint) error {
	file, err := os.OpenFile(filepath.Join(dir, "endpoint.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("cannot publish supervisor endpoint")
	}
	err = json.NewEncoder(file).Encode(endpoint)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func readEndpoint(dir string) (supervisorEndpoint, error) {
	var endpoint supervisorEndpoint
	if err := checkPrivatePath(dir, true); err != nil {
		return endpoint, err
	}
	path := filepath.Join(dir, "endpoint.json")
	if err := checkPrivatePath(path, false); err != nil {
		return endpoint, err
	}
	file, err := os.Open(path)
	if err != nil {
		return endpoint, errors.New("cannot read supervisor endpoint")
	}
	defer file.Close()
	if json.NewDecoder(io.LimitReader(file, 8192)).Decode(&endpoint) != nil {
		return endpoint, errors.New("invalid supervisor endpoint")
	}
	host, _, err := net.SplitHostPort(endpoint.Address)
	if err != nil || host != "127.0.0.1" || endpoint.Token == "" || endpoint.Instance == "" {
		return endpoint, errors.New("invalid supervisor endpoint")
	}
	return endpoint, nil
}
