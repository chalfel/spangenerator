package main

import (
	"context"
	"flag"
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
func InjectSpans(root string) error {
	// Walk through all the files in the project directory
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Process only Go source files, skipping test files if desired
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			if err := processFile(path); err != nil {
				log.Printf("Failed to process file %s: %v", path, err)
			}
		}
		return nil
	})

	return err
}

// processFile modifies a .go file to add spans to functions
func processFile(filename string) error {
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
			if fn.Body != nil && !strings.HasPrefix(fn.Name.Name, "init") {
				// Create new tracing logic to add at the start of the function
				tracingStmt := &ast.ExprStmt{
					X: &ast.CallExpr{
						Fun:  ast.NewIdent("StartSpanFromContext"),
						Args: []ast.Expr{ast.NewIdent("ctx"), ast.NewIdent(`otel.Tracer("exampleTracer")`), ast.NewIdent(`"` + fn.Name.Name + `"`)},
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
	return false
}

func main() {
	root := flag.String("root", ".", "Root directory to apply span injection")
	flag.Parse()

	if root == nil || *root == "" {
		log.Fatal("Root directory is required")
		return
	}

	// Call the library function
	err := InjectSpans(*root)
	if err != nil {
		log.Fatal(err)
	}
}
