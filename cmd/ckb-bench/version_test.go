//go:build cartographer

package main

import (
	"fmt"
	"github.com/SimplyLiz/CodeMCP/internal/cartographer"
)

func init() {
	v, err := cartographer.Version()
	fmt.Printf("Cartographer version: %q err=%v\n", v, err)
}
