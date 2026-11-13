package hoststate

import "errors"

func Acquire(string) (func(), error) {
	return nil, errors.New("native Windows host ownership is unsupported; use WSL2")
}
