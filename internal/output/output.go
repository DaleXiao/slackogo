package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
)

type Format int

const (
	FormatHuman Format = iota
	FormatPlain
	FormatJSON
)

type Printer struct {
	Format Format
	Writer io.Writer
}

func NewPrinter(format Format) *Printer {
	return &Printer{Format: format, Writer: os.Stdout}
}

// JSON outputs structured JSON
func (p *Printer) JSON(v any) error {
	enc := json.NewEncoder(p.Writer)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Plain outputs tab-separated fields
func (p *Printer) Plain(fields ...string) {
	fmt.Fprintln(p.Writer, strings.Join(fields, "\t"))
}

// Human outputs colored human-readable text
func (p *Printer) Human(format string, a ...any) {
	fmt.Fprintf(p.Writer, format+"\n", a...)
}

// Header prints a colored header
func (p *Printer) Header(s string) {
	if p.Format == FormatHuman {
		color.New(color.FgCyan, color.Bold).Fprintln(p.Writer, s)
	}
}

// Success prints green text
func (p *Printer) Success(format string, a ...any) {
	if p.Format == FormatHuman {
		color.New(color.FgGreen).Fprintf(p.Writer, format+"\n", a...)
	}
}

// Error prints red text to stderr
func (p *Printer) Error(format string, a ...any) {
	color.New(color.FgRed).Fprintf(os.Stderr, format+"\n", a...)
}

// Auto outputs based on format setting
func (p *Printer) Auto(jsonData any, plainFields [][]string, humanFn func()) error {
	switch p.Format {
	case FormatJSON:
		return p.JSON(jsonData)
	case FormatPlain:
		for _, fields := range plainFields {
			p.Plain(fields...)
		}
		return nil
	default:
		humanFn()
		return nil
	}
}
