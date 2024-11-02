package spangenerator

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/tools/go/ast/astutil"
)

// StartSpanFromContext is the function to be injected for tracing purposes.
func StartSpanFromContext(ctx context.Context, tracer trace.Tracer, receiver interface{}) (context.Context, trace.Span) {
	// Get the caller's function name
	pc, _, _, _ := runtime.Caller(1)
	fullFuncName := runtime.FuncForPC(pc).Name()

	// Extract just the function name
	funcName := fullFuncName[strings.LastIndex(fullFuncName, ".")+1:]

	// Get the struct name if it's a method call on a struct
	structName := ""
	if receiver != nil {
		structName = reflect.TypeOf(receiver).Elem().Name()
	}

	// Create a combined name for the span
	spanName := structName + "." + funcName

	// Start the span with the combined name
	return tracer.Start(ctx, spanName)
}

// InjectSpans walks through all .go files in the specified root directory
// and injects span creation code into all functions.
func InjectSpans(root string, tracerName string) error {
	// Walk through all the files in the project directory
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Process only Go source files, skipping test files if desired
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") && info.Name() != "main.go" {
			if err := processFile(path, tracerName); err != nil {
				log.Printf("Failed to process file %s: %v", path, err)
			}
		}
		return nil
	})

	return err
}

// processFile modifies a .go file to add spans to functions
func processFile(filename string, tracerName string) error {
	// Open the file and parse it
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.AllErrors)
	if err != nil {
		return err
	}

	// Traverse the AST to find functions and modify them
	ast.Inspect(file, func(n ast.Node) bool {
		// Find function declarations
		if fn, ok := n.(*ast.FuncDecl); ok {
			if fn.Body != nil && hasContextParameter(fn) && !strings.HasPrefix(fn.Name.Name, "init") {
				// Create new tracing logic to add at the start of the function
				tracingStmt := &ast.AssignStmt{
					Lhs: []ast.Expr{
						ast.NewIdent("ctx"),
						ast.NewIdent("_"),
					},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{
						&ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   ast.NewIdent("spangenerator"),
								Sel: ast.NewIdent("StartSpanFromContext"),
							},
							Args: []ast.Expr{
								ast.NewIdent("ctx"),
								ast.NewIdent(fmt.Sprintf(`otel.Tracer("%s")`, tracerName)),
								&ast.BasicLit{
									Kind:  token.STRING,
									Value: `"` + fn.Name.Name + `"`,
								},
							},
						},
					},
				}
				// Insert the tracing logic at the beginning of the function body
				fn.Body.List = append([]ast.Stmt{tracingStmt}, fn.Body.List...)
			}
		}
		return true
	})

	// Ensure the import statement for the tracing function is added
	if !hasImport(file, "github.com/chalfel/spangenerator") {
		astutil.AddImport(fset, file, "github.com/chalfel/spangenerator")
	}

	if !hasImport(file, "go.opentelemetry.io/otel") {
		astutil.AddImport(fset, file, "go.opentelemetry.io/otel")
	}

	// Write the modified file back
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := printer.Fprint(f, fset, file); err != nil {
		return err
	}

	log.Printf("Successfully modified file %s", filename)
	return nil
}

// hasImport checks if the given import is already present in the file
func hasImport(file *ast.File, pkg string) bool {
	for _, i := range file.Imports {
		if i.Path.Value == `"`+pkg+`"` {
			return true
		}
	}

	// If there are no functions that need span injection, return true to indicate no import is needed
	hasFunctionToInject := false
	ast.Inspect(file, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			if fn.Body != nil && !strings.HasPrefix(fn.Name.Name, "init") {
				hasFunctionToInject = true
				return false // No need to continue inspecting
			}
		}
		return true
	})

	return !hasFunctionToInject
}

func hasContextParameter(fn *ast.FuncDecl) bool {
	for _, param := range fn.Type.Params.List {
		if selectorExpr, ok := param.Type.(*ast.SelectorExpr); ok {
			if ident, ok := selectorExpr.X.(*ast.Ident); ok && ident.Name == "context" {
				if selectorExpr.Sel.Name == "Context" {
					return true
				}
			}
		}
	}
	return false
}
