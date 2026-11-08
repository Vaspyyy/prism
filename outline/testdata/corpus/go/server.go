package server

import (
	"fmt"
	"strings"
)

// Usage is printed by -h. The text has lines that look like code.
const Usage = `server: serve the files

func notAFunction() {
type NotAType struct{}
`

/*
func commentedOut() error {
	return nil
}
*/

// Server serves files.
type Server struct {
	root string
}

// Start starts the server.
func (s *Server) Start(addr string) error {
	query := `
	func insideQuery() {}
	`
	_ = query
	return fmt.Errorf("not yet: %s", addr)
}

func (s Server) Root() string { return s.root }

// Handler is generic over its value.
func Handler[T any](v T) string {
	return fmt.Sprint(v) /* func inline() {} */
}

type handlerFunc func(string) string

func run() {
	tmpl := `{{define "x"}}
type Template struct{}
{{end}}`
	_ = strings.TrimSpace(tmpl)
}

func run2() { run() }

const winRoot = `C:\`

func afterBackslash() string { return winRoot }

var (
	defaultRoot = "."
	maxConns    = 8
)
