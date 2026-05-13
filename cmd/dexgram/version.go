package main

import (
	"fmt"
	"io"
)

const appVersion = "0.1.16"

func printVersion(w io.Writer) {
	_, _ = fmt.Fprintf(w, "Dexgram %s\n", appVersion)
}
