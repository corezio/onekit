package gengo

import (
	"fmt"
	"go/format"
	"strings"

	"github.com/1homsi/onekit/internal/onkir"
)

type Printer struct {
	b        strings.Builder
	resolver PackageResolver
}

func newPrinter(resolver PackageResolver) *Printer {
	return &Printer{resolver: resolver}
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

// MessageTypeName returns the Go type name to use when referencing m from
// the file currently being printed: the bare name if m belongs to this same
// generated package, or an import-qualified name (e.g. "otherpkg.Name") if it
// belongs to a different one (see PackageResolver).
func (p *Printer) MessageTypeName(m *onkir.Message) string {
	if p.resolver != nil {
		if ref, ok := p.resolver.ResolveMessage(m); ok {
			return ref.Alias + "." + m.Name
		}
	}
	return m.Name
}

// EnumTypeName is MessageTypeName's counterpart for enums.
func (p *Printer) EnumTypeName(e *onkir.Enum) string {
	if p.resolver != nil {
		if ref, ok := p.resolver.ResolveEnum(e); ok {
			return ref.Alias + "." + e.Name
		}
	}
	return e.Name
}

// GoFieldType is GoFieldType, but resolves Message/Enum kinds through this
// printer's PackageResolver so cross-package field types get an
// import-qualified name instead of a bare (and possibly wrong) local one.
func (p *Printer) GoFieldType(t *onkir.Type) string {
	switch t.Kind {
	case onkir.KindScalar:
		return GoScalarType(t.Scalar)
	case onkir.KindMessage:
		return "*" + p.MessageTypeName(t.Message)
	case onkir.KindEnum:
		return p.EnumTypeName(t.Enum)
	case onkir.KindMap:
		return "map[" + GoScalarType(t.MapKey) + "]" + p.GoFieldType(t.MapValue)
	default:
		return "any"
	}
}
