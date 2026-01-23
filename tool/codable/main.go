package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

type codableType struct {
	Name     string
	Copyable bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "codable: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fileFlag := flag.String("file", "", "go file containing //go:generate codable")
	flag.Parse()

	fileName := strings.TrimSpace(*fileFlag)
	if fileName == "" && flag.NArg() > 0 {
		fileName = strings.TrimSpace(flag.Arg(0))
	}
	if fileName == "" {
		fileName = strings.TrimSpace(os.Getenv("GOFILE"))
	}
	if fileName == "" {
		return errors.New("missing source file; set GOFILE or pass -file")
	}
	fileName = filepath.Base(fileName)
	if filepath.Ext(fileName) != ".go" {
		return fmt.Errorf("source file must be a .go file: %s", fileName)
	}

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles,
		Dir: dir,
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src, parser.ParseComments)
		},
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return errors.New("no packages found")
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return fmt.Errorf("type check failed: %s", pkg.Errors[0])
	}
	if pkg.Fset == nil {
		return errors.New("missing fileset")
	}
	if len(pkg.Syntax) == 0 {
		return errors.New("no go files found in package")
	}

	var targetFile *ast.File
	for i, file := range pkg.Syntax {
		var name string
		if i < len(pkg.CompiledGoFiles) {
			name = pkg.CompiledGoFiles[i]
		} else if i < len(pkg.GoFiles) {
			name = pkg.GoFiles[i]
		}
		if filepath.Base(name) == fileName {
			targetFile = file
			break
		}
	}
	if targetFile == nil {
		return fmt.Errorf("file %s not found in package", fileName)
	}

	typesToGenerate, err := collectCodableTypes(targetFile, pkg.TypesInfo, pkg.Fset)
	if err != nil {
		return err
	}
	if len(typesToGenerate) == 0 {
		return fmt.Errorf("no codable structs found in %s", fileName)
	}

	out, err := render(pkg.Name, typesToGenerate)
	if err != nil {
		return err
	}

	base := strings.TrimSuffix(fileName, ".go")
	outPath := filepath.Join(dir, base+"_codable.go")
	return os.WriteFile(outPath, out, 0o644)
}

func collectCodableTypes(file *ast.File, info *types.Info, fset *token.FileSet) ([]codableType, error) {
	var results []codableType
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if !commentGroupHasCodable(typeSpec.Doc) && !commentGroupHasCodable(gen.Doc) {
				continue
			}
			if _, ok := typeSpec.Type.(*ast.StructType); !ok {
				pos := fset.Position(typeSpec.Pos())
				return nil, fmt.Errorf("codable requires struct type at %s", pos)
			}

			obj := info.Defs[typeSpec.Name]
			if obj == nil {
				pos := fset.Position(typeSpec.Pos())
				return nil, fmt.Errorf("missing type info for %s at %s", typeSpec.Name.Name, pos)
			}
			name, ok := obj.(*types.TypeName)
			if !ok {
				pos := fset.Position(typeSpec.Pos())
				return nil, fmt.Errorf("expected type name for %s at %s", typeSpec.Name.Name, pos)
			}

			copyable := isCopyable(name.Type(), make(map[types.Type]bool), make(map[types.Type]bool))
			results = append(results, codableType{Name: typeSpec.Name.Name, Copyable: copyable})
		}
	}

	return results, nil
}

func commentGroupHasCodable(group *ast.CommentGroup) bool {
	if group == nil {
		return false
	}
	for _, comment := range group.List {
		for _, line := range splitCommentLines(comment.Text) {
			if isCodableDirective(line) {
				return true
			}
		}
	}
	return false
}

func splitCommentLines(text string) []string {
	text = strings.TrimSpace(text)
	switch {
	case strings.HasPrefix(text, "//"):
		line := strings.TrimSpace(strings.TrimPrefix(text, "//"))
		return []string{line}
	case strings.HasPrefix(text, "/*"):
		body := strings.TrimSuffix(strings.TrimPrefix(text, "/*"), "*/")
		lines := strings.Split(body, "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "*") {
				line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
			}
			lines[i] = line
		}
		return lines
	default:
		return []string{text}
	}
}

func isCodableDirective(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "go:generate") {
		return false
	}
	fields := strings.Fields(line)
	return len(fields) >= 2 && fields[1] == "codable"
}

func isCopyable(t types.Type, cache map[types.Type]bool, visiting map[types.Type]bool) bool {
	if t == nil {
		return false
	}
	if val, ok := cache[t]; ok {
		return val
	}
	if visiting[t] {
		return false
	}
	visiting[t] = true

	var result bool
	switch tt := t.(type) {
	case *types.Basic:
		if tt.Info()&types.IsString != 0 || tt.Kind() == types.UnsafePointer {
			result = false
			break
		}
		result = true
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Interface, *types.Signature:
		result = false
	case *types.Array:
		result = isCopyable(tt.Elem(), cache, visiting)
	case *types.Struct:
		result = true
		for i := 0; i < tt.NumFields(); i++ {
			if !isCopyable(tt.Field(i).Type(), cache, visiting) {
				result = false
				break
			}
		}
	case *types.Named:
		result = isCopyable(tt.Underlying(), cache, visiting)
	default:
		result = false
	}

	cache[t] = result
	delete(visiting, t)
	return result
}

func render(pkgName string, types []codableType) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("// Code generated by codable; DO NOT EDIT.\n\n")
	fmt.Fprintf(&buf, "package %s\n\n", pkgName)

	needUnsafe := false
	for _, t := range types {
		if t.Copyable {
			needUnsafe = true
			break
		}
	}
	if needUnsafe {
		buf.WriteString("import \"unsafe\"\n\n")
	}

	for i, t := range types {
		if i > 0 {
			buf.WriteString("\n")
		}
		if t.Copyable {
			writeCopyable(&buf, t.Name)
			continue
		}
		writeStub(&buf, t.Name)
	}

	out, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, err
	}
	return out, nil
}

func writeCopyable(buf *bytes.Buffer, typeName string) {
	recv := receiverName(typeName)
	fmt.Fprintf(buf, "func (%s %s) SizeInByte() int {\n", recv, typeName)
	fmt.Fprintf(buf, "\treturn int(unsafe.Sizeof(%s))\n", recv)
	fmt.Fprintf(buf, "}\n\n")

	fmt.Fprintf(buf, "func (%s %s) Encode(dst []byte) []byte {\n", recv, typeName)
	fmt.Fprintf(buf, "\tsize := %s.SizeInByte()\n", recv)
	fmt.Fprintf(buf, "\tif cap(dst) < size {\n")
	fmt.Fprintf(buf, "\t\tdst = make([]byte, size)\n")
	fmt.Fprintf(buf, "\t} else {\n")
	fmt.Fprintf(buf, "\t\tdst = dst[:size]\n")
	fmt.Fprintf(buf, "\t}\n\n")
	fmt.Fprintf(buf, "\tsrc := unsafe.Slice((*byte)(unsafe.Pointer(&%s)), size)\n", recv)
	fmt.Fprintf(buf, "\tcopy(dst, src)\n")
	fmt.Fprintf(buf, "\treturn dst\n")
	fmt.Fprintf(buf, "}\n\n")

	fmt.Fprintf(buf, "func (%s) Decode(src []byte) %s {\n", typeName, typeName)
	fmt.Fprintf(buf, "\tvar result %s\n", typeName)
	fmt.Fprintf(buf, "\tsize := int(unsafe.Sizeof(result))\n")
	fmt.Fprintf(buf, "\tdst := unsafe.Slice((*byte)(unsafe.Pointer(&result)), size)\n")
	fmt.Fprintf(buf, "\tcopy(dst, src)\n")
	fmt.Fprintf(buf, "\treturn result\n")
	fmt.Fprintf(buf, "}\n")
}

func writeStub(buf *bytes.Buffer, typeName string) {
	recv := receiverName(typeName)
	fmt.Fprintf(buf, "func (%s %s) SizeInByte() int {\n", recv, typeName)
	fmt.Fprintf(buf, "\treturn 0\n")
	fmt.Fprintf(buf, "}\n\n")

	fmt.Fprintf(buf, "func (%s %s) Encode(dst []byte) []byte {\n", recv, typeName)
	fmt.Fprintf(buf, "\treturn nil\n")
	fmt.Fprintf(buf, "}\n\n")

	fmt.Fprintf(buf, "func (%s %s) Decode(src []byte) %s {\n", recv, typeName, typeName)
	fmt.Fprintf(buf, "\tvar result %s\n", typeName)
	fmt.Fprintf(buf, "\treturn result\n")
	fmt.Fprintf(buf, "}\n")
}

func receiverName(typeName string) string {
	if typeName == "" {
		return "v"
	}
	r := strings.ToLower(typeName[:1])
	if len(r) == 0 {
		return "v"
	}
	if r[0] < 'a' || r[0] > 'z' {
		return "v"
	}
	return r
}
