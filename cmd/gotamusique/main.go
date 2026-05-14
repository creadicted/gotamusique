package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	fmt.Fprintf(os.Stdout, "gotamusique v%s\n", version)
}
