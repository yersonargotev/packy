package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type Terminal interface {
	Interactive(io.Reader) bool
	Approve(io.Reader, io.Writer, string) (bool, error)
}

type processTerminal struct{}

func (processTerminal) Interactive(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (processTerminal) Approve(in io.Reader, out io.Writer, prompt string) (bool, error) {
	if _, err := fmt.Fprintf(out, "%s [y/N] ", prompt); err != nil {
		return false, err
	}
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}
