package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
)

type supervisorEndpoint struct {
	Address   string `json:"address"`
	Token     string `json:"token"`
	Instance  string `json:"instance"`
	Workspace string `json:"workspace"`
}

func claimSupervisorState(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New("cannot create private supervisor directory")
	}
	if err := checkPrivatePath(dir, true); err != nil {
		return nil, err
	}
	lock := filepath.Join(dir, "owner.lock")
	file, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errors.New("supervisor state already owned; do not delete locks of a live supervisor")
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(lock)
		return nil, err
	}
	if _, err := os.Lstat(filepath.Join(dir, "endpoint.json")); !os.IsNotExist(err) {
		_ = os.Remove(lock)
		return nil, errors.New("existing supervisor endpoint; verify old supervisor stopped before manually recovering stale state")
	}
	return func() { _ = os.Remove(filepath.Join(dir, "endpoint.json")); _ = os.Remove(lock) }, nil
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
