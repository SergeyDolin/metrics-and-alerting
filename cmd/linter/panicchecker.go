package main

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name:     "panicchecker",
	Doc:      "reports calls to panic(), and log.Fatal/os.Exit outside of main.main",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// run is the main analysis function.
func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Precompute main function positions
	mainFuncs := findMainFunctions(pass)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	inspect.Preorder(nodeFilter, func(n ast.Node) {
		call := n.(*ast.CallExpr)

		// Check for panic
		checkPanic(pass, call)

		// Check for log.Fatal and os.Exit
		checkFatalExit(pass, call, mainFuncs)
	})

	return nil, nil
}

// findMainFunctions locates all main function declarations.
func findMainFunctions(pass *analysis.Pass) map[*ast.FuncDecl]bool {
	mainFuncs := make(map[*ast.FuncDecl]bool)

	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			if fd, ok := n.(*ast.FuncDecl); ok && fd.Name.Name == "main" {
				mainFuncs[fd] = true
			}
			return true
		})
	}

	return mainFuncs
}

// isInMainNode checks if a node is inside a main function.
func isInMainNode(pass *analysis.Pass, n ast.Node, mainFuncs map[*ast.FuncDecl]bool) bool {
	if pass.Pkg.Name() != "main" {
		return false
	}

	// Get the position of the node
	nodePos := pass.Fset.Position(n.Pos())

	// Check each main function
	for mainFunc := range mainFuncs {
		start := pass.Fset.Position(mainFunc.Pos())
		end := pass.Fset.Position(mainFunc.End())
		if start.Filename == nodePos.Filename &&
			nodePos.Line >= start.Line &&
			nodePos.Line <= end.Line {
			return true
		}
	}

	return false
}

// checkPanic examines panic calls.
func checkPanic(pass *analysis.Pass, call *ast.CallExpr) {
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "panic" {
		return
	}

	obj := pass.TypesInfo.ObjectOf(fun)
	if obj == nil || obj.Parent() != types.Universe {
		return
	}

	pass.Reportf(call.Pos(), "use of built-in panic function is discouraged")
}

// checkFatalExit examines log.Fatal and os.Exit calls.
func checkFatalExit(pass *analysis.Pass, call *ast.CallExpr, mainFuncs map[*ast.FuncDecl]bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return
	}

	var funcName string
	switch {
	case pkg.Name == "log" && sel.Sel.Name == "Fatal":
		funcName = "log.Fatal"
	case pkg.Name == "os" && sel.Sel.Name == "Exit":
		funcName = "os.Exit"
	default:
		return
	}

	// If not in main package - always report
	if pass.Pkg.Name() != "main" {
		pass.Reportf(call.Pos(), "call to %s outside of main package", funcName)
		return
	}

	// Check if we're in main function
	if !isInMainNode(pass, call, mainFuncs) {
		pass.Reportf(call.Pos(), "call to %s outside of main() function", funcName)
	}
}
