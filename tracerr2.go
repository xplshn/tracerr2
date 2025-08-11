// Package tracerr provides an error type that includes a stack trace
// with formatted source code context and syntax highlighting.
package tracerr

import (
	"bufio"
	"bytes"
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

// Error represents an error with an associated stack trace.
type Error struct {
	Msg    string  // The error message.
	Frames []Frame // The stack trace frames.
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

// Error returns the error message without the stack trace, satisfying the error interface.
func (e *Error) Error() string {
	return e.Msg
}

// Print prints the error message and stack trace to os.Stderr.
func (e *Error) Print() {
	e.Fprint(os.Stderr)
}

// Fprint formats and writes the error and stack trace to the given writer.
// It includes the error message, stack frames, and highlighted source code context.
func (e *Error) Fprint(w io.Writer) {
	fmt.Fprintf(w, "%s%s%s\n", red(e.Msg), colorReset, "")
	for _, frame := range e.Frames {
		// Format the frame information (e.g., "at main.Gamma (main.go:123)")
		location := gray(fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line))
		function := yellow(frame.Function)
		fmt.Fprintf(w, "  at %s (%s)\n", function, location)

		// Read the source code lines around the error
		lines, startLine, err := readSourceContextLines(frame.File, frame.Line, 1)
		if err != nil {
			fmt.Fprintf(w, "    %s\n", gray("Could not read source file"))
			continue
		}

		// Join lines to be highlighted by chroma
		codeBlock := strings.Join(lines, "\n")
		var highlightedBuf bytes.Buffer
		// Highlight the code block using chroma for better readability
		err = quick.Highlight(&highlightedBuf, codeBlock, "go", "terminal256", "monokai")
		if err != nil {
			// Fallback to non-highlighted if chroma fails
			highlightedBuf.WriteString(codeBlock)
		}
		highlightedLines := strings.Split(highlightedBuf.String(), "\n")

		// Calculate width for line numbers for proper alignment
		lineNumWidth := len(fmt.Sprintf("%d", startLine+len(lines)-1))
		errorLineIndex := frame.Line - startLine

		// Print the formatted, highlighted source context
		for i, hLine := range highlightedLines {
			if i >= len(lines) { // Ensure we don't go out of bounds
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

			if isErrorLine {
				caretGutter := boldGray("  " + strings.Repeat(" ", lineNumWidth) + " | ")
				// This assumes simple character alignment, which is usually fine for code
				caretLine := red(strings.Repeat("^", len(lines[i])))
				fmt.Fprintf(w, "%s%s\n", caretGutter, caretLine)
			}
		}
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
