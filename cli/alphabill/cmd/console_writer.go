package cmd

import "fmt"

// consoleWriter global variable used to print output to console,
// used for capturing console output in tests
var consoleWriter = newStdoutWriter()

type (
	consoleWrapper interface {
		Println(a ...any)
		Print(a ...any)
	}

	stdoutWrapper struct {
	}
)

func newStdoutWriter() consoleWrapper {
	return &stdoutWrapper{}
}

func (w *stdoutWrapper) Println(a ...any) {
	fmt.Println(a...)
}

func (w *stdoutWrapper) Print(a ...any) {
	fmt.Print(a...)
}
