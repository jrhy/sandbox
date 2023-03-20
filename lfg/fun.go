package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

func main() {
	fset := token.NewFileSet()                         // positions are relative to fset
	f, err := parser.ParseFile(fset, "fun.go", nil, 0) //parser.ImportsOnly)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("(package %s)\n\n", Symbolize(f.Name.Name))
	if len(f.Imports) > 0 {
		fmt.Printf("(import \n")
		for _, i := range f.Imports {
			if i.Path == nil {
				if i.Name != nil {
					fmt.Printf("  (%s ???)\n", Symbolize(i.Name.Name))
				}
				continue
			}
			if i.Name != nil {
				fmt.Printf("  (%s %s)\n", Symbolize(i.Name.Name), Symbolize(i.Path))
			} else {
				fmt.Printf("  %s\n", Symbolize(i.Path))
			}
		}
		fmt.Printf(")\n\n")
	}

	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.GenDecl:
			for i := range d.Specs {
				fmt.Printf("(spec %T\n", d.Specs[i])
			}
		case *ast.FuncDecl:
			if d.Recv != nil {
				fmt.Printf("(define-method (%s %s) (%s", d.Recv.List[0].Names[0], d.Recv.List[0].Type, d.Name)
			}
			fmt.Printf("(define (%s", d.Name)
			for _, f := range d.Type.Params.List {
				name := f.Names[0].Name
				t := ParamType(f.Type)
				fmt.Printf(" %s %s", name, t)
			}
			fmt.Printf(") (")
			if d.Type.Results != nil {
				for i, f := range d.Type.Results.List {
					name := "_"
					if len(f.Names) == 1 {
						name = f.Names[0].Name
					}
					t := ParamType(f.Type)
					if i > 0 {
						fmt.Printf(" ")
					}
					fmt.Printf("%s %s", name, t)
				}
			}
			fmt.Printf(") (...))\n")
		default:
			fmt.Printf("(%T)\n", d)
		}
	}
}

func ParamType(i interface{}) string {
	switch x := i.(type) {
	case *ast.InterfaceType:
		if x.Incomplete {
			return fmt.Sprintf("TODO-incomplete")
		} else if len(x.Methods.List) == 0 {
			return "'()"
		} else {
			return fmt.Sprintf("TODO-%d-methods", len(x.Methods.List))
		}
	case *ast.Ident:
		return SymbolizeString(x.Name)
	}
	return fmt.Sprintf("unknown-%T", i)
}
func Symbolize(i interface{}) string {
	switch s := i.(type) {
	case *ast.BasicLit:
		switch s.Kind {
		case token.STRING:
			if len(s.Value) > 0 {
				if s.Value[0] != '"' {
					return fmt.Sprintf(`"TODO: %c-string"`, s.Value[0])
				}
				return SymbolizeString(s.Value[1 : len(s.Value)-1])
			} else {
				return `""`
			}
		}
		return fmt.Sprintf(`"TODO: token.%d"`, s.Kind)
	case string:
		return SymbolizeString(s)
	}
	return fmt.Sprintf(`"TODO: %T"`, i)
}
func SymbolizeString(s string) string {
	m, err := regexp.Match(`^[a-z0-9_/]*$`, []byte(s))
	if err != nil {
		panic(err)
	}
	if m {
		return "'" + s
	}
	if strings.Contains(s, "\"") {
		return "'TODO" + s
	}
	return "\"" + s + "\""
}
