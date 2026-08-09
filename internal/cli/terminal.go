package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

type Terminal interface {
	Interactive(io.Reader) bool
	InteractiveSession(io.Reader, io.Writer) bool
	Approve(io.Reader, io.Writer, string) (bool, error)
}

type processTerminal struct{}

func (processTerminal) Interactive(in io.Reader) bool {
	input, ok := in.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(input.Fd())
}

func (processTerminal) InteractiveSession(in io.Reader, out io.Writer) bool {
	input, inputOK := in.(*os.File)
	output, outputOK := out.(*os.File)
	if !inputOK || !outputOK {
		return false
	}
	return term.IsTerminal(input.Fd()) && term.IsTerminal(output.Fd())
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
