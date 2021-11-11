package logger

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Hackish way to get the goroutine id - taken from goksi
func goroutineID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

// Returns caller with last two directories.
// Meant for using in console for development - not performance optimized.
func consoleFormatCallerLastTwoDirs(i interface{}) string {
	var c string
	if cc, ok := i.(string); ok {
		c = cc
	}
	if len(c) > 0 {
		split := strings.Split(c, string(os.PathSeparator))
		l := len(split)
		if l > 2 {
			c = fmt.Sprintf("%s/%s/%s", split[l-3], split[l-2], split[l-1])
		} else if l > 1 {
			c = fmt.Sprintf("%s/%s", split[l-2], split[l-1])
		}
	}
	return c
}
