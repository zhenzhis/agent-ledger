package collector

import (
	"bufio"
	"io"
)

const (
	jsonlInitialBufferSize = 1024 * 1024
	jsonlMaxLineSize       = 10 * 1024 * 1024
)

func newJSONLScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, jsonlInitialBufferSize), jsonlMaxLineSize)
	return scanner
}
