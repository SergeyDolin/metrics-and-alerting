package main

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the main entry point for the panicchecker static analysis tool.
// It defines the analyzer's metadata and the run function that performs the analysis.
// The analyzer reports two categories of issues:
//  1. Direct calls to the built-in panic() function anywhere in the code.
//  2. Calls to log.Fatal or os.Exit that occur outside of the main() function
//     in the main package.
var Analyzer = &analysis.Analyzer{
	Name:     "panicchecker",
	Doc:      "reports calls to panic(), and log.Fatal/os.Exit outside of main.main",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// run is the main analysis function. It traverses the Abstract Syntax Tree (AST)
// of the analyzed package and reports any detected violations.
//
// It uses the inspector.Analyzer for efficient AST traversal and precomputes
// the locations of main() functions to optimize the checking process.
func run(pass *analysis.Pass) (interface{}, error) {
	// Retrieve the inspector from the analysis pass results.
	// The inspector provides a faster, filtered way to traverse the AST.
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Precompute the positions of all main() functions in the package.
	// This optimization prevents repeated AST walks when checking each call expression.
	mainFuncs := findMainFunctions(pass)

	// Define a filter to only traverse function call expressions.
	// This significantly improves performance by focusing only on nodes of interest.
	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	// Traverse all call expressions in the package.
	inspect.Preorder(nodeFilter, func(n ast.Node) {
		call := n.(*ast.CallExpr)

		// Check for direct calls to the built-in panic() function.
		checkPanic(pass, call)

		// Check for calls to log.Fatal or os.Exit.
		checkFatalExit(pass, call, mainFuncs)
	})

	return nil, nil
}

// findMainFunctions locates all function declarations named "main" within the package.
// It returns a map where the keys are the AST nodes representing main functions.
//
// This precomputation is necessary for efficiently determining whether a given
// call expression is inside a main() function.
func findMainFunctions(pass *analysis.Pass) map[ast.Node]bool {
	mainFuncs := make(map[ast.Node]bool)

	// Iterate through all files in the package.
	for _, file := range pass.Files {
		// Inspect each file to find function declarations.
		ast.Inspect(file, func(n ast.Node) bool {
			// Check if the node is a function declaration named "main".
			if fd, ok := n.(*ast.FuncDecl); ok && fd.Name.Name == "main" {
				mainFuncs[fd] = true
			}
			return true
		})
	}

	return mainFuncs
}

// isInMainNode determines whether a given AST node is located inside any
// main() function in the package.
//
// Parameters:
//   - pass: The analysis pass containing type and position information.
//   - n: The AST node to check.
//   - mainFuncs: A precomputed map of main function nodes.
//
// Returns:
//   - bool: true if the node is inside a main() function, false otherwise.
func isInMainNode(pass *analysis.Pass, n ast.Node, mainFuncs map[ast.Node]bool) bool {
	// If the current package is not "main", the node cannot be in a main() function.
	if pass.Pkg.Name() != "main" {
		return false
	}

	// Attempt to traverse up the node hierarchy to find enclosing functions.
	for node := n; node != nil; {
		// If we encounter a function declaration, check if it's a main function.
		if fd, ok := node.(*ast.FuncDecl); ok {
			return mainFuncs[fd]
		}

		// If we encounter an anonymous function literal, we're definitely not in main().
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}

		// Note: Direct parent traversal is not easily available.
		// Fall back to position-based checking below.
		break
	}

	// Fallback: Use position information to determine if the node is within
	// the source code boundaries of any main() function.
	pos := pass.Fset.Position(n.Pos())

	for mainFunc := range mainFuncs {
		start := pass.Fset.Position(mainFunc.Pos())
		end := pass.Fset.Position(mainFunc.End())

		// Check if the node's position falls within the line range of a main function
		// in the same file.
		if start.Filename == pos.Filename &&
			pos.Line >= start.Line &&
			(end.Line == 0 || pos.Line <= end.Line) {
			return true
		}
	}

	return false
}

// checkPanic examines a call expression to determine if it's a direct call
// to the built-in panic() function. If so, it reports a diagnostic.
//
// Parameters:
//   - pass: The analysis pass for reporting diagnostics.
//   - call: The call expression to examine.
func checkPanic(pass *analysis.Pass, call *ast.CallExpr) {
	// Check if the called function is a simple identifier named "panic".
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "panic" {
		return
	}

	// Verify that this identifier refers to the built-in panic function
	// from the universe scope, not a user-defined function.
	obj := pass.TypesInfo.ObjectOf(fun)
	if obj == nil || obj.Parent() != types.Universe {
		return
	}

	// Report the violation.
	pass.Reportf(call.Pos(), "use of built-in panic function is discouraged")
}

// checkFatalExit examines a call expression to determine if it's a call to
// log.Fatal or os.Exit. If such a call is found outside of a main() function
// in the main package, it reports a diagnostic.
//
// Parameters:
//   - pass: The analysis pass for reporting diagnostics.
//   - call: The call expression to examine.
//   - mainFuncs: A precomputed map of main function nodes.
func checkFatalExit(pass *analysis.Pass, call *ast.CallExpr, mainFuncs map[ast.Node]bool) {
	// Check if the call is of the form pkg.Func (selector expression).
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	// Extract the package identifier.
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return
	}

	// Determine the full function name.
	var funcName string
	switch {
	case pkg.Name == "log" && sel.Sel.Name == "Fatal":
		funcName = "log.Fatal"
	case pkg.Name == "os" && sel.Sel.Name == "Exit":
		funcName = "os.Exit"
	default:
		return // Not a function we're interested in.
	}

	// If we're not in the main package, always report (these functions
	// can prematurely terminate the entire program).
	if pass.Pkg.Name() != "main" {
		pass.Reportf(call.Pos(), "call to %s outside of main package", funcName)
		return
	}

	// We're in the main package, so only report if the call is outside
	// the main() function itself.
	if !isInMainNode(pass, call, mainFuncs) {
		pass.Reportf(call.Pos(), "call to %s outside of main() function", funcName)
	}
}
