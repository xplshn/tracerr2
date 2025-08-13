// Package tracerr provides an error type that includes a stack trace
// with formatted source code context and syntax highlighting.
package tracerr

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
)

// ANSI color and formatting constants for terminal output.
const (
	colorRed      = "\033[31m"
	colorYellow   = "\033[33m"
	colorReset    = "\033[0m"
	colorGray     = "\033[90m"
	colorBoldGray = "\033[1;90m"
	formatItalic  = "\033[3m"
)

// Frame represents a single frame in a stack trace.
type Frame struct {
	File     string // The file path of the frame.
	Line     int    // The line number in the file.
	Function string // The name of the function.
}

// Error represents an error with an associated stack trace and a potential cause.
type Error struct {
	Msg    string  // The error message.
	Frames []Frame // The stack trace frames.
	cause  error   // The wrapped error.
}

// New creates a new Tracerr error with a message and a stack trace.
// It captures the stack trace at the point it is called.
func New(msg string) *Error {
	return newError(msg, 2)
}

// Errorf creates a new Tracerr error with a formatted message and a stack trace.
// It captures the stack trace at the point it is called.
func Errorf(format string, args ...interface{}) *Error {
	return newError(fmt.Sprintf(format, args...), 2)
}

// Wrap annotates an existing error with a new message and a stack trace.
// If err is nil, Wrap returns nil. The original error is preserved.
func Wrap(err error, msg string) *Error {
	if err == nil {
		return nil
	}
	e := newError(msg, 2)
	e.cause = err
	return e
}

// Wrapf annotates an existing error with a new formatted message and a stack trace.
// If err is nil, Wrapf returns nil. The original error is preserved.
func Wrapf(err error, format string, args ...interface{}) *Error {
	if err == nil {
		return nil
	}
	e := newError(fmt.Sprintf(format, args...), 2)
	e.cause = err
	return e
}

// newError is the internal helper to create an error and capture the stack.
// The 'skip' parameter indicates how many stack frames to ascend.
func newError(msg string, skip int) *Error {
	frames := make([]Frame, 0, 10)
	for i := skip; ; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		fn := runtime.FuncForPC(pc)
		var funcName string
		if fn != nil {
			funcName = filepath.Base(fn.Name())
		} else {
			funcName = "<unknown>"
		}
		// Stop capturing frames when we reach the Go runtime entry points.
		if strings.HasPrefix(funcName, "runtime.") {
			break
		}
		frames = append(frames, Frame{
			File:     file,
			Line:     line,
			Function: funcName,
		})
	}
	return &Error{
		Msg:    msg,
		Frames: frames,
	}
}

// Error returns the error message, including messages from wrapped errors.
func (e *Error) Error() string {
	if e.cause != nil {
		return e.Msg + ": " + e.cause.Error()
	}
	return e.Msg
}

// Unwrap returns the wrapped error, allowing compatibility with standard errors.Is and errors.As.
func (e *Error) Unwrap() error {
	return e.cause
}

// Print prints the error message and stack trace to os.Stderr.
func (e *Error) Print() {
	e.Fprint(os.Stderr)
}

// Fprint formats and writes the full error chain and stack traces to the given writer.
// It includes the error message, stack frames, and highlighted source code context for each error in the chain.
func (e *Error) Fprint(w io.Writer) {
	var currentErr error = e
	isFirst := true

	for currentErr != nil {
		// Check if the current error in the chain is a *tracerr.Error
		tracerrErr, ok := currentErr.(*Error)

		if !isFirst {
			fmt.Fprintf(w, "\n%sCaused by: %s", formatItalic, colorReset)
		}

		if ok {
			// It's a tracerr error, print its message and stack trace.
			fmt.Fprintf(w, "%s\n", red(tracerrErr.Msg))
			for _, frame := range tracerrErr.Frames {
				printFrame(w, frame)
			}
		} else {
			// It's a standard error, just print its message.
			fmt.Fprintf(w, "%s\n", red(currentErr.Error()))
		}

		// Move to the next error in the chain.
		currentErr = errors.Unwrap(currentErr)
		isFirst = false
	}
}

// printFrame formats and prints a single stack frame with source code context.
func printFrame(w io.Writer, frame Frame) {
	location := gray(fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line))
	function := yellow(frame.Function)
	fmt.Fprintf(w, "  at %s (%s)\n", function, location)

	lines, startLine, err := readSourceContextLines(frame.File, frame.Line, 1)
	if err != nil {
		fmt.Fprintf(w, "    %s\n", gray("Could not read source file"))
		return
	}

	codeBlock := strings.Join(lines, "\n")
	var highlightedBuf bytes.Buffer
	err = quick.Highlight(&highlightedBuf, codeBlock, "go", "terminal256", "monokai")
	if err != nil {
		highlightedBuf.WriteString(codeBlock)
	}
	highlightedLines := strings.Split(highlightedBuf.String(), "\n")

	lineNumWidth := len(fmt.Sprintf("%d", startLine+len(lines)-1))
	errorLineIndex := frame.Line - startLine

	for i, hLine := range highlightedLines {
		if i >= len(lines) {
			continue
		}
		lineNum := startLine + i
		isErrorLine := i == errorLineIndex

		var gutter string
		if isErrorLine {
			gutter = boldGray(fmt.Sprintf("  %*d | ", lineNumWidth, lineNum))
		} else {
			gutter = gray(fmt.Sprintf("  %*d | ", lineNumWidth, lineNum))
		}

		fmt.Fprintf(w, "%s%s\n", gutter, hLine)
	}
}

// readSourceContextLines reads a specified number of lines of context from a file
// around a central line number. It returns the lines, the starting line number, and an error.
func readSourceContextLines(filePath string, centerLine, context int) ([]string, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	start := centerLine - context
	if start < 1 {
		start = 1
	}
	end := centerLine + context

	var lines []string
	scanner := bufio.NewScanner(file)
	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine >= start && currentLine <= end {
			lines = append(lines, scanner.Text())
		}
		if currentLine > end {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	if len(lines) == 0 {
		return nil, 0, fmt.Errorf("no lines found in range")
	}

	return lines, start, nil
}

// ANSI formatting helpers wrap strings with ANSI escape codes
func italic(s string) string   { return formatItalic + s + colorReset }
func gray(s string) string     { return colorGray + s + colorReset }
func boldGray(s string) string { return colorBoldGray + s + colorReset }
func red(s string) string      { return colorRed + s + colorReset }
func yellow(s string) string   { return colorYellow + s + colorReset }
