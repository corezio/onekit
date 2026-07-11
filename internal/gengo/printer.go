package gengo

import (
	"fmt"
	"go/format"
	"strings"
)

type Printer struct {
	b strings.Builder
}

func (p *Printer) P(args ...any) {
	for _, a := range args {
		fmt.Fprint(&p.b, a)
	}
	p.b.WriteByte('\n')
}

func (p *Printer) Format() ([]byte, error) {
	return format.Source([]byte(p.b.String()))
}

func (p *Printer) Raw() string {
	return p.b.String()
}
