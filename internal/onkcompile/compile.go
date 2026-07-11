package onkcompile

import (
	"fmt"

	"github.com/1homsi/onekit/internal/onkir"
	"github.com/1homsi/onekit/internal/onklang"
)

type Error struct {
	Path string
	Line int
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Msg)
}

type Source struct {
	Path string
	AST  *onklang.File
}

type compiler struct {
	msgByName  map[string]*onkir.Message
	enumByName map[string]*onkir.Enum
	msgNode    map[*onklang.MessageDecl]*onkir.Message
	enumNode   map[*onklang.EnumDecl]*onkir.Enum
}

func Compile(sources []Source) (*onkir.Package, error) {
	c := &compiler{
		msgByName:  map[string]*onkir.Message{},
		enumByName: map[string]*onkir.Enum{},
		msgNode:    map[*onklang.MessageDecl]*onkir.Message{},
		enumNode:   map[*onklang.EnumDecl]*onkir.Enum{},
	}

	var files []*onkir.File
	for _, src := range sources {
		f := &onkir.File{Path: src.Path, Package: src.AST.Package, Imports: src.AST.Imports}
		files = append(files, f)

		for _, md := range src.AST.Messages {
			m, err := c.declareMessage(md, f, nil, src.Path)
			if err != nil {
				return nil, err
			}
			f.Messages = append(f.Messages, m)
		}
		for _, ed := range src.AST.Enums {
			e, err := c.declareEnum(ed, f, nil, src.Path)
			if err != nil {
				return nil, err
			}
			f.Enums = append(f.Enums, e)
		}
	}

	for i, src := range sources {
		f := files[i]
		for _, md := range src.AST.Messages {
			if err := c.fillMessage(md, src.Path); err != nil {
				return nil, err
			}
		}
		for _, sd := range src.AST.Services {
			s, err := c.buildService(sd, f, src.Path)
			if err != nil {
				return nil, err
			}
			f.Services = append(f.Services, s)
		}
	}

	return &onkir.Package{Files: files}, nil
}

func (c *compiler) declareMessage(md *onklang.MessageDecl, f *onkir.File, parent *onkir.Message, path string) (*onkir.Message, error) {
	if _, exists := c.msgByName[md.Name]; exists {
		return nil, &Error{Path: path, Line: md.Line, Msg: fmt.Sprintf("duplicate message name %q", md.Name)}
	}
	if _, exists := c.enumByName[md.Name]; exists {
		return nil, &Error{Path: path, Line: md.Line, Msg: fmt.Sprintf("name %q already used by an enum", md.Name)}
	}

	m := &onkir.Message{Name: md.Name, Doc: md.Doc, File: f, Parent: parent}
	m.Decorators = convertDecorators(md.Decorators)
	c.msgByName[md.Name] = m
	c.msgNode[md] = m

	for _, nested := range md.Nested {
		nm, err := c.declareMessage(nested, f, m, path)
		if err != nil {
			return nil, err
		}
		m.Nested = append(m.Nested, nm)
	}
	for _, nested := range md.NestedEn {
		ne, err := c.declareEnum(nested, f, m, path)
		if err != nil {
			return nil, err
		}
		m.NestedEnums = append(m.NestedEnums, ne)
	}

	return m, nil
}

func (c *compiler) declareEnum(ed *onklang.EnumDecl, f *onkir.File, parent *onkir.Message, path string) (*onkir.Enum, error) {
	if _, exists := c.enumByName[ed.Name]; exists {
		return nil, &Error{Path: path, Line: ed.Line, Msg: fmt.Sprintf("duplicate enum name %q", ed.Name)}
	}
	if _, exists := c.msgByName[ed.Name]; exists {
		return nil, &Error{Path: path, Line: ed.Line, Msg: fmt.Sprintf("name %q already used by a message", ed.Name)}
	}

	e := &onkir.Enum{Name: ed.Name, Doc: ed.Doc, File: f, Parent: parent}
	c.enumByName[ed.Name] = e
	c.enumNode[ed] = e

	for i, vd := range ed.Values {
		e.Values = append(e.Values, &onkir.EnumValue{
			Name:       vd.Name,
			Doc:        vd.Doc,
			Decorators: convertDecorators(vd.Decorators),
			Enum:       e,
			Index:      i,
		})
	}

	return e, nil
}

func (c *compiler) fillMessage(md *onklang.MessageDecl, path string) error {
	m := c.msgNode[md]
	for _, fd := range md.Fields {
		field, err := c.buildField(fd, m, path)
		if err != nil {
			return err
		}
		m.Fields = append(m.Fields, field)
	}
	for _, nested := range md.Nested {
		if err := c.fillMessage(nested, path); err != nil {
			return err
		}
	}
	return nil
}

func (c *compiler) buildField(fd *onklang.FieldDecl, owner *onkir.Message, path string) (*onkir.Field, error) {
	field := &onkir.Field{
		Name:       fd.Name,
		Doc:        fd.Doc,
		Repeated:   fd.Repeated,
		Optional:   fd.Optional,
		Decorators: convertDecorators(fd.Decorators),
		Message:    owner,
	}

	if fd.Oneof != nil {
		oneof, err := c.buildOneof(fd.Oneof, field, path)
		if err != nil {
			return nil, err
		}
		field.Oneof = oneof
		return field, nil
	}

	typ, err := c.resolveType(fd.Type, path, fd.Line)
	if err != nil {
		return nil, err
	}
	field.Type = typ
	return field, nil
}

func (c *compiler) buildOneof(od *onklang.OneofDecl, field *onkir.Field, path string) (*onkir.Oneof, error) {
	oneof := &onkir.Oneof{Field: field, Args: convertArgs(od.Args)}
	for _, vd := range od.Variants {
		typ, err := c.resolveType(vd.Type, path, vd.Line)
		if err != nil {
			return nil, err
		}
		oneof.Variants = append(oneof.Variants, &onkir.OneofVariant{
			Name:       vd.Name,
			Type:       typ,
			Decorators: convertDecorators(vd.Decorators),
			Oneof:      oneof,
		})
	}
	return oneof, nil
}

func (c *compiler) resolveType(t *onklang.TypeRef, path string, line int) (*onkir.Type, error) {
	if t.IsMap {
		keyKind, ok := onkir.ParseScalarKind(t.MapKey)
		if !ok {
			return nil, &Error{Path: path, Line: line, Msg: fmt.Sprintf("invalid map key type %q", t.MapKey)}
		}
		val, err := c.resolveType(t.MapVal, path, line)
		if err != nil {
			return nil, err
		}
		return &onkir.Type{Kind: onkir.KindMap, MapKey: keyKind, MapValue: val}, nil
	}

	if scalar, ok := onkir.ParseScalarKind(t.Name); ok {
		return &onkir.Type{Kind: onkir.KindScalar, Scalar: scalar}, nil
	}
	if m, ok := c.msgByName[t.Name]; ok {
		return &onkir.Type{Kind: onkir.KindMessage, Message: m}, nil
	}
	if e, ok := c.enumByName[t.Name]; ok {
		return &onkir.Type{Kind: onkir.KindEnum, Enum: e}, nil
	}
	return nil, &Error{Path: path, Line: line, Msg: fmt.Sprintf("unresolved type %q", t.Name)}
}

func (c *compiler) buildService(sd *onklang.ServiceDecl, f *onkir.File, path string) (*onkir.Service, error) {
	s := &onkir.Service{Name: sd.Name, Doc: sd.Doc, BasePath: sd.BasePath, File: f}
	headers, err := c.buildHeaders(sd.Headers, path)
	if err != nil {
		return nil, err
	}
	s.Headers = headers

	for _, rd := range sd.RPCs {
		method, err := c.buildMethod(rd, s, path)
		if err != nil {
			return nil, err
		}
		s.Methods = append(s.Methods, method)
	}
	return s, nil
}

func (c *compiler) buildMethod(rd *onklang.RPCDecl, s *onkir.Service, path string) (*onkir.Method, error) {
	req, ok := c.msgByName[rd.RequestType]
	if !ok {
		return nil, &Error{Path: path, Line: rd.Line, Msg: fmt.Sprintf("unresolved request type %q", rd.RequestType)}
	}
	resp, ok := c.msgByName[rd.ResponseType]
	if !ok {
		return nil, &Error{Path: path, Line: rd.Line, Msg: fmt.Sprintf("unresolved response type %q", rd.ResponseType)}
	}
	headers, err := c.buildHeaders(rd.Headers, path)
	if err != nil {
		return nil, err
	}

	method := &onkir.Method{
		Name:       rd.Name,
		Doc:        rd.Doc,
		Request:    req,
		Response:   resp,
		Decorators: convertDecorators(rd.Decorators),
		Headers:    headers,
		Service:    s,
	}

	for _, errName := range rd.ErrorTypes {
		errMsg, ok := c.msgByName[errName]
		if !ok {
			return nil, &Error{Path: path, Line: rd.Line, Msg: fmt.Sprintf("unresolved error type %q", errName)}
		}
		method.ErrorTypes = append(method.ErrorTypes, errMsg)
	}

	return method, nil
}

func (c *compiler) buildHeaders(headers []onklang.HeaderDecl, path string) ([]*onkir.Header, error) {
	var out []*onkir.Header
	for _, h := range headers {
		kind, ok := onkir.ParseScalarKind(h.Type)
		if !ok {
			return nil, &Error{Path: path, Line: h.Line, Msg: fmt.Sprintf("invalid header type %q for %q", h.Type, h.Name)}
		}
		out = append(out, &onkir.Header{
			Name:       h.Name,
			Type:       kind,
			Decorators: convertDecorators(h.Decorators),
		})
	}
	return out, nil
}

func convertDecorators(decorators []onklang.Decorator) []onkir.Decorator {
	var out []onkir.Decorator
	for _, d := range decorators {
		out = append(out, onkir.Decorator{Name: d.Name, Args: convertArgs(d.Args)})
	}
	return out
}

func convertArgs(args []onklang.Arg) []onkir.Arg {
	var out []onkir.Arg
	for _, a := range args {
		out = append(out, onkir.Arg{Name: a.Name, Value: a.Value})
	}
	return out
}
