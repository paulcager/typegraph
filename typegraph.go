package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/types"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"golang.org/x/tools/go/loader"
)

type structType struct {
	pkg      *types.Package
	Name     string // E.g. "MyType"
	FullName string // E.g. "my/pkg.MyType"
	PkgName  string // E.g. "pkg"
	Fields   []field
	Depth    int
	Orphan   bool
}

func (s *structType) Colour() string {
	//if s.Orphan {
	//	return "#c0c0c0"
	//}
	return "#e0e0ff"
}

// Types to pass into the graphvis templates.

// A field within a type.
type field struct {
	Name     string // E.g. "Thing"
	Type     types.Type
	TypeName string // E.g. "MyType"
	FullType string // E.g. "my/package.MyType"
}

// A reference from a field in one type to another type.
type useInfo struct {
	FromStruct, FromField, ToStruct string
}

// To parse command args.
type stringArgs []string

func (s *stringArgs) String() string {
	return strings.Join(*s, ",")
}
func (s *stringArgs) Set(value string) error {
	*s = append(*s, value)
	return nil
}

var (
	pkgNames []string
	conf     = loader.Config{ParserMode: parser.ParseComments}
	prog     *loader.Program
	structs  = make(map[string]*structType)
)

func main() {
	var (
		err            error
		include        stringArgs
		exclude        stringArgs
		includeOrphans bool
	)

	flag.BoolVar(&includeOrphans, "includeOrphans", false, "Include orphan types")
	flag.Var(&include, "include", "include if referenced")
	flag.Var(&exclude, "exclude", "always exclude")
	flag.Parse()

	for _, s := range include {
		addInclusion(s)
	}
	for _, s := range exclude {
		addExclusion(s)
	}

	pkgNames = flag.Args()
	if len(flag.Args()) == 0 {
		pkgNames = []string{"."}
	}

	if _, err := conf.FromArgs(pkgNames, false); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	prog, err = conf.Load()
	must(err, "%s", err)

	for _, pkg := range prog.Imported {
		fmt.Println("pkg", pkg)
		findStructs(pkg)
	}

	if len(structs) == 0 {
		abort("No structures to print")
	}

	for x := range structs {
		fmt.Println(structs[x].FullName, len(structs[x].Fields))
	}

	var links []useInfo
	nonOrphans := make(map[string]bool)
	for _, str := range structs {
		for i := range str.Fields {
			f := str.Fields[i]
			names := findNamedTypes(f.Type)
			for _, name := range names {
				fullName := name.String()
				if _, include := structs[fullName]; include {
					nonOrphans[str.FullName] = true
					nonOrphans[fullName] = true
					links = append(links, useInfo{FromStruct: str.FullName, FromField: f.Name, ToStruct: fullName})
				}
			}
		}
	}

	out, err := os.Create("test.dot")
	must(err, "")
	defer out.Close()

	hdrTmpl.Execute(out, map[string]string{"Cmd": strings.Join(pkgNames, " ")})

	for _, str := range structs {
		str.Orphan = !nonOrphans[str.FullName]
		if includeOrphans || !str.Orphan {
			structTmpl.Execute(out, str)
		}
	}

	for _, l := range links {
		arrowTmpl.Execute(out, l)
	}

	tailTmpl.Execute(out, nil)
	out.Close()

	cmd := exec.Command("dot", "-Tsvg", "-O", "test.dot")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err = cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run %s.\n%s\n", cmd.Args, err)
		os.Exit(2)
	}
}

// findNamedTypes follows the given type, collecting Named Types it may reference.
// For example "[]*time.Time" would return {time.Time}
// "map[time.Time]time.Duration" would return {time.Time, time.Duration}
func findNamedTypes(t types.Type) []*types.Named {
	switch t := t.(type) {
	case *types.Named:
		return []*types.Named{t}
	case *types.Array:
		return findNamedTypes(t.Elem())
	case *types.Chan:
		return findNamedTypes(t.Elem())
	case *types.Map:
		ret := findNamedTypes(t.Key())
		ret = append(ret, findNamedTypes(t.Elem())...)
		return ret
	case *types.Pointer:
		return findNamedTypes(t.Elem())
	case *types.Slice:
		return findNamedTypes(t.Elem())
	}

	return nil
}

// findStructs returns a map of structure types in the package, or to be included from a referenced package.
func findStructs(pkg *loader.PackageInfo) {
	for _, def := range pkg.Defs {
		if typName, ok := def.(*types.TypeName); ok {
			if typName.Parent() != pkg.Pkg.Scope() {
				// Not at package-level scope, e.g. defined locally in a func.
				continue
			}
			uType := typName.Type().(*types.Named).Underlying()
			addStruct(0, pkg.Pkg, typName, uType)
		}
	}
}

func addStruct(depth int, pkg *types.Package, typName *types.TypeName, underlying types.Type) {
	fullName := pkg.Path() + "." + typName.Name()
	if s := structs[fullName]; s != nil {
		return
	}

	if !include(pkg.Path(), typName.Name()) {
		return
	}

	pkgParts := strings.Split(pkg.Path(), "/")
	pkgName := pkgParts[len(pkgParts)-1]
	switch str := underlying.(type) {
	case *types.Struct:
		st := &structType{
			pkg:      pkg,
			Name:     typName.Name(),
			FullName: fullName,
			PkgName:  pkgName,
			Depth:    depth,
		}
		n := str.NumFields()
		for i := 0; i < n; i++ {
			f := str.Field(i)
			st.Fields = append(st.Fields, field{
				Name:     f.Name(),
				TypeName: types.TypeString(f.Type(), types.RelativeTo(st.pkg)),
				FullType: f.Type().String(),
				Type:     f.Type(),
			})
		}
		structs[fullName] = st
		follow(depth+1, pkg, typName, str)
	case *types.Interface:
		fmt.Println("Iface", typName.Name(), str.String())

		st := &structType{
			pkg:      pkg,
			Name:     typName.Name(),
			FullName: fullName,
			PkgName:  pkgName,
			Depth:    depth,
		}
		n := str.NumMethods()
		for i := 0; i < n; i++ {
			m := str.Method(i)
			st.Fields = append(st.Fields, field{
				Name:     m.Name(),
				TypeName: types.TypeString(m.Type(), types.RelativeTo(st.pkg)),
				FullType: m.Type().String(),
				Type:     m.Type(),
			})
		}

		structs[fullName] = st
	}
	return
}

func follow(depth int, sourcePkg *types.Package, typName *types.TypeName, str *types.Struct) {
	for i := 0; i < str.NumFields(); i++ {
		f := str.Field(i)
		names := findNamedTypes(f.Type())
		for _, name := range names {
			pkg := name.Obj().Pkg()
			if pkg == nil {
				continue // A built-in type, such as error
			}

			uType := name.Underlying()
			addStruct(depth, pkg, name.Obj(), uType)
		}
	}
}

func must(err error, format string, params ...interface{}) {
	if err != nil {
		abort(format, params...)
	}
}

func abort(format string, params ...interface{}) {
	fmt.Fprintf(os.Stderr, format, params...)
	os.Exit(2)
}

var hdrTmpl = template.Must(template.New("hdr").Parse(`digraph "structDiagram" {
  graph [
    rankdir="LR"
    label="\nGenerated by typegraph: {{.Cmd}}"
    URL="http://www.github.com/paulcager/typegraph"
    labeljust="l"
    //ranksep="0.5"
    //nodesep="0.2"
    fontsize="12"
    fontname="Helvetica"
    bgcolor="#f8f8f8"
  ];
  node [
    fontname="Helvetica"
    fontsize="12"
    shape="plaintext"
  ];
  edge [
    arrowsize="1.5"
  ];
`))

var structTmpl = template.Must(template.New("struct").Parse(`"{{.FullName}}" [
    label=<
    <TABLE BORDER="0" CELLBORDER="1" CELLSPACING="0" CELLPADDING="2" BGCOLOR="#ffffff">
      <TR><TD COLSPAN="2" BGCOLOR="{{.Colour}}" ALIGN="CENTER">{{.PkgName}}.{{.Name}} </TD></TR>
      {{range .Fields}}      <TR><TD PORT="X{{.Name}}" COLSPAN="1" BGCOLOR="#f0f0ff" ALIGN="LEFT">{{.Name}} </TD> <TD PORT="{{.Name}}" COLSPAN="1" BGCOLOR="#f0f0ff" ALIGN="LEFT">{{html .TypeName}} </TD></TR>
      {{end}}
    </TABLE>>
    tooltip="{{.FullName}}"
  ];
`))

var arrowTmpl = template.Must(template.New("arrow").Parse(`  "{{.FromStruct}}":"{{.FromField}}" -> "{{.ToStruct}}" [];
`))

var tailTmpl = template.Must(template.New("tail").Parse(`
}
`))
