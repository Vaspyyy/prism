//go:build !unix

package outline

import "errors"

func mkfifo(string) error { return errors.New("not supported") }
