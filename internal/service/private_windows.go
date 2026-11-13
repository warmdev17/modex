package service

import "errors"

func privateState(string) error { return errors.New("use WSL2 for host services") }
