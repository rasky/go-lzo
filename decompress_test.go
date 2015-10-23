package lzo

import (
	"strings"
	"testing"
)

func TestDecompCrasher1(t *testing.T) {
	Decompress1X(strings.NewReader("\x00"), 0, 0)
}

func TestDecompCrasher2(t *testing.T) {
	Decompress1X(strings.NewReader("\x00\x030000000000000000000000\x01\x000\x000"), 0, 0)
}
