package main

import (
	"fmt"
	"go/types"
	"log"

	"golang.org/x/tools/go/loader"
)

func main() {
	g := &generator{}
	err := g.Load("go/types")
	if err != nil {
		panic(err)
	}
	g.ScanType("go/types", "object")
	for k, v := range g.structs {
		fmt.Println(k)
		for _, field := range v.fields {
			fmt.Println("  ", field.typeRef != nil, field.name, field.typeString)
		}
	}
}

type Field struct {
	name       string
	typeString string
	typeRef    *Struct
}
type Struct struct {
	name   string
	fields []Field
}

type generator struct {
	loader  loader.Config
	prog    *loader.Program
	structs map[string]*Struct
}

func (g *generator) Load(pkg ...string) error {
	var err error
	for _, s := range pkg {
		g.loader.Import(s)
	}
	g.prog, err = g.loader.Load()
	g.structs = make(map[string]*Struct)
	return err
}

func (g *generator) ScanType(pkg, typ string) {
	p := g.prog.Package(pkg)
	if p == nil {
		panic("Package " + pkg + " not loaded")
	}
	obj := p.Pkg.Scope().Lookup(typ)
	if obj == nil {
		panic("Type " + pkg + "." + typ + " not loaded")
	}

	g.scan(typ, followType(obj.Type()))
}

func (g *generator) scan(typeName string, typ types.Type) *Struct {
	s, found := g.structs[typeName]
	if found {
		return s
	}

	log.Println("Scanning " + typ.String())

	switch t := typ.(type) {
	case *types.Struct:
		s = &Struct{name: typeName}
		g.structs[typeName] = s
		for i := 0; i < t.NumFields(); i++ {
			fieldType := t.Field(i).Type()
			fieldName := t.Field(i).Name()
			s.fields = append(s.fields, Field{
				name:       fieldName,
				typeString: fieldType.String(),
				typeRef:    g.scan(fieldName, followType(fieldType)),
			})
		}
	}

	return nil
}

func followType(typ types.Type) types.Type {
	for {
		switch t := typ.(type) {
		case *types.Named:
			typ = t.Underlying()
		case *types.Pointer:
			typ = t.Elem()
		case *types.Array:
			typ = t.Elem()
		case *types.Slice:
			typ = t.Elem()
		default:
			return t
		}
	}
}
