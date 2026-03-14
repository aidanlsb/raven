package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type checkInteraction interface {
	Printf(format string, args ...interface{})
	Println(args ...interface{})
	ReadLine() (string, error)
}

type stdioCheckInteraction struct {
	reader *bufio.Reader
	out    io.Writer
}

func newCheckInteraction(in io.Reader, out io.Writer) checkInteraction {
	return &stdioCheckInteraction{
		reader: bufio.NewReader(in),
		out:    out,
	}
}

func (i *stdioCheckInteraction) Printf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(i.out, format, args...)
}

func (i *stdioCheckInteraction) Println(args ...interface{}) {
	_, _ = fmt.Fprintln(i.out, args...)
}

func (i *stdioCheckInteraction) ReadLine() (string, error) {
	line, err := i.reader.ReadString('\n')
	if err == io.EOF && line != "" {
		return line, nil
	}
	return line, err
}

func readTrimmedLine(interaction checkInteraction) string {
	line, _ := interaction.ReadLine()
	return strings.TrimSpace(line)
}

func readTrimmedLowerLine(interaction checkInteraction) string {
	return strings.ToLower(readTrimmedLine(interaction))
}
