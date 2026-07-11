package gents

import (
	"fmt"
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

func (p *Printer) Bytes() []byte {
	return []byte(p.b.String())
}
