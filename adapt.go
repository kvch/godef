package main

// This file adapts between the two implementations of godef and should be removed when
// we fully switch to the go/packages implementation for all cases.

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	gotoken "go/token"
	gotypes "go/types"

	rpast "github.com/kvch/godef/go/ast"
	rpprinter "github.com/kvch/godef/go/printer"
	rptypes "github.com/kvch/godef/go/types"
	"golang.org/x/tools/go/packages"
)

var forcePackages triBool

func init() {
	flag.Var(&forcePackages, "new-implementation", "force godef to use the new go/packages implementation")
}

// triBool represents a tri-state boolean flag (unset, on, off)
type triBool int

const (
	unset triBool = iota // Unset state
	on                   // On state
	off                  // Off state
)

func (b *triBool) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	if v {
		*b = on
	} else {
		*b = off
	}
	return nil
}

func (b *triBool) Get() interface{} {
	return *b
}

func (b *triBool) String() string {
	switch *b {
	case unset:
		return "default"
	case on:
		return "true"
	case off:
		return "false"
	default:
		return "invalid"
	}
}

func (b *triBool) IsBoolFlag() bool {
	return true
}

// detectModuleMode detects if module mode is enabled based on the packages.Config.
func detectModuleMode(cfg *packages.Config) bool {
	for _, e := range cfg.Env {
		switch e {
		case "GO111MODULE=off":
			return false
		case "GO111MODULE=on":
			return true
		}
	}
	if _, err := os.Stat(filepath.Join(cfg.Dir, "go.mod")); !os.IsNotExist(err) {
		return true
	}
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Env = cfg.Env
	cmd.Dir = cfg.Dir
	out, err := cmd.Output()
	if err == nil {
		return len(strings.TrimSpace(string(out))) > 0
	}
	return false
}

// adaptGodef adapts between godef implementations based on the provided config.
func adaptGodef(cfg *packages.Config, filename string, src []byte, searchpos int) (*Object, error) {
	usePackages := false
	switch forcePackages {
	case unset:
		usePackages = detectModuleMode(cfg)
	case on:
		usePackages = true
	case off:
		usePackages = false
	}
	if usePackages {
		fset, obj, err := godefPackages(cfg, filename, src, searchpos)
		if err != nil {
			return nil, err
		}
		return adaptGoObject(fset, obj)
	}
	obj, typ, err := godef(filename, src, searchpos)
	if err != nil {
		return nil, err
	}
	return adaptRPObject(obj, typ)
}

// adaptRPObject adapts an rpObject to an Object.
func adaptRPObject(obj *rpast.Object, typ rptypes.Type) (*Object, error) {
	pos := rptypes.FileSet.Position(rptypes.DeclPos(obj))
	result := &Object{
		Name: obj.Name,
		Pkg:  typ.Pkg,
		Position: Position{
			Filename: pos.Filename,
			Line:     pos.Line,
			Column:   pos.Column,
		},
		Type: typ,
	}

	switch obj.Kind {
	case rpast.Bad:
		result.Kind = BadKind
	case rpast.Fun:
		result.Kind = FuncKind
	case rpast.Var:
		result.Kind = VarKind
	case rpast.Pkg:
		result.Kind = ImportKind
		result.Type = nil
		if typ.Node != nil {
			result.Value = typ.Node.(*rpast.ImportSpec).Path.Value
		} else {
			result.Kind = PathKind
			result.Value = obj.Data.(string)
		}
	case rpast.Con:
		result.Kind = ConstKind
		if decl, ok := obj.Decl.(*rpast.ValueSpec); ok {
			result.Value = decl.Values[0]
		}
	case rpast.Lbl:
		result.Kind = LabelKind
		result.Type = nil
	case rpast.Typ:
		result.Kind = TypeKind
		result.Type = typ.Underlying(false)
	}

	for child := range typ.Iter() {
		m, err := adaptRPObject(child, rptypes.Type{})
		if err != nil {
			return nil, err
		}
		result.Members = append(result.Members, m)
	}
	sort.Sort(orderedObjects(result.Members))
	return result, nil
}

// adaptGoObject adapts a go/types.Object to an Object.
func adaptGoObject(fset *gotoken.FileSet, obj gotypes.Object) (*Object, error) {
	result := &Object{
		Name:     obj.Name(),
		Position: objToPos(fset, obj),
		Type:     obj.Type(),
	}

	switch obj := obj.(type) {
	case *gotypes.Func:
		result.Kind = FuncKind
	case *gotypes.Var:
		result.Kind = VarKind
	case *gotypes.PkgName:
		result.Kind = ImportKind
		result.Type = nil
		if obj.Pkg() != nil {
			result.Value = strconv.Quote(obj.Imported().Path())
		} else {
			result.Value = obj.Imported().Path()
			result.Kind = PathKind
		}
	case *gotypes.Const:
		result.Kind = ConstKind
		result.Value = obj.Val()
	case *gotypes.Label:
		result.Kind = LabelKind
		result.Type = nil
	case *gotypes.TypeName:
		result.Kind = TypeKind
		result.Type = obj.Type().Underlying()
	default:
		result.Kind = BadKind
	}

	return result, nil
}

// objToPos converts a go/types.Object position to a Position struct.
func objToPos(fSet *gotoken.FileSet, obj gotypes.Object) Position {
	p := obj.Pos()
	f := fSet.File(p)
	goPos := f.Position(p)
	pos := Position{
		Filename: cleanFilename(goPos.Filename),
		Line:     goPos.Line,
		Column:   goPos.Column,
	}
	if pos.Column != 1 {
		return pos
	}
	named, ok := obj.(interface{ Name() string })
	if !ok {
		return pos
	}
	in, err := os.Open(f.Name())
	if err != nil {
		return pos
	}
	defer in.Close()
	for l, scanner := 1, bufio.NewScanner(in); scanner.Scan(); l++ {
		if l < pos.Line {
			continue
		}
		col := bytes.Index([]byte(scanner.Text()), []byte(named.Name()))
		if col >= 0 {
			pos.Column = col + 1
		}
		break
	}
	return pos
}

// cleanFilename normalizes any file names that come out of the fileset.
func cleanFilename(path string) string {
	const prefix = "$GOROOT"
	if len(path) < len(prefix) || !strings.EqualFold(prefix, path[:len(prefix)]) {
		return path
	}
	return runtime.GOROOT() + path[len(prefix):]
}

// pretty is a type that implements custom formatting for different types.
type pretty struct {
	n interface{}
}

func (p pretty) Format(f fmt.State, c rune) {
	switch n := p.n.(type) {
	case *rpast.BasicLit:
		rpprinter.Fprint(f, rptypes.FileSet, n)
	case rptypes.Type:
		rpprinter.Fprint(f, rptypes.FileSet, n.Node)
	case gotypes.Type:
		buf := &bytes.Buffer{}
		gotypes.WriteType(buf, n, func(p *gotypes.Package) string { return "" })
		buf.WriteTo(f)
	default:
		fmt.Fprint(f, n)
	}
}
