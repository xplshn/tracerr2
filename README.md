## tracerr2

A Go error tracing library that provides detailed, syntax-highlighted stack traces

### Usage

```go
package main

import (
	"fmt"

	"github.com/xplshn/tracerr2"
)

// Example usage demonstrates how to create and print a tracerr error.
func main() {
	err := PinkFloyd()
	if err != nil {
		// Type assert to custom error to access the Print method
		if e, ok := err.(*tracerr.Error); ok {
			e.Print()
		} else {
			fmt.Println(err)
		}
	}
}

func PinkFloyd() error {
	return Alpha()
}

func Alpha() error {
	return Beta()
}

func Beta() error {
	return Gamma()
}

func Gamma() error {
	return tracerr.Errorf("something went wrong here")
}
```
