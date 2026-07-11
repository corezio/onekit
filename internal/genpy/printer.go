package genpy

import (
	"fmt"
	"strings"
)

type Printer struct {
	b      strings.Builder
	indent int
}

func (p *Printer) P(args ...any) {
	p.b.WriteString(strings.Repeat("    ", p.indent))
	for _, a := range args {
		fmt.Fprint(&p.b, a)
	}
	p.b.WriteByte('\n')
}

func (p *Printer) Indent() {
	p.indent++
}

func (p *Printer) Dedent() {
	if p.indent > 0 {
		p.indent--
	}
}

func (p *Printer) Blank() {
	p.b.WriteByte('\n')
}

func (p *Printer) Bytes() []byte {
	return []byte(p.b.String())
}
