package testtoolchain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type goTestCommandIssue struct {
	line    int
	command string
}

// The repository guard finds every Go-toolchain test subprocess and requires its command variable to be explicitly marked.
func TestEveryGoToolchainTestCommandInheritsLock(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var issues []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root {
				switch entry.Name() {
				case ".git", ".temp", "vendor":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, issue := range goTestCommandInheritanceIssues(path, source) {
			relative, _ := filepath.Rel(root, path)
			issues = append(issues, relative+":"+strconv.Itoa(issue.line)+": missing InheritHostGOROOTLock for "+issue.command)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("Go-toolchain test subprocesses missing explicit inheritance marks:\n%s", strings.Join(issues, "\n"))
	}
}

// The guard rejects the exact missing-marker defect and accepts its marked neighbor.
func TestGoToolchainTestCommandGuardPlants(t *testing.T) {
	const prefix = `package sample
import (
  "os/exec"
  "testing"
  "github.com/relux-works/archon/internal/testtoolchain"
)
func TestPlant(t *testing.T) {
  command := exec.Command(testtoolchain.Go(t), "test", "./...")
  _ = command
}
`
	issues := goTestCommandInheritanceIssues("plant_test.go", []byte(prefix))
	if len(issues) != 1 || issues[0].command != "command" {
		t.Fatalf("unmarked plant was not identified precisely: %#v", issues)
	}
	marked := strings.Replace(prefix, "  _ = command", "  testtoolchain.InheritHostGOROOTLock(t, command)\n  _ = command", 1)
	if issues := goTestCommandInheritanceIssues("valid_test.go", []byte(marked)); len(issues) != 0 {
		t.Fatalf("marked valid neighbor was rejected: %#v", issues)
	}
	const inline = `package sample
import (
  "os/exec"
  "testing"
  "github.com/relux-works/archon/internal/testtoolchain"
)
func TestPlant(t *testing.T) {
  _ = exec.Command(testtoolchain.Go(t), "test", "./...")
}
`
	issues = goTestCommandInheritanceIssues("inline_plant_test.go", []byte(inline))
	if len(issues) != 1 || issues[0].command != "<unassigned>" {
		t.Fatalf("inline unmarked plant was not identified: %#v", issues)
	}
}

func goTestCommandInheritanceIssues(filename string, source []byte) []goTestCommandIssue {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, source, 0)
	if err != nil {
		return []goTestCommandIssue{{command: "parse error: " + err.Error()}}
	}
	execAliases, toolchainAliases := importedPackageAliases(file)
	var issues []goTestCommandIssue
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		goTools := map[string]bool{}
		testArgs := map[string]bool{}
		commandNames := map[*ast.CallExpr]string{}
		var commandCalls []*ast.CallExpr
		markedNames := map[string]bool{}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.AssignStmt:
				for index, rhs := range value.Rhs {
					if index >= len(value.Lhs) {
						break
					}
					name, ok := value.Lhs[index].(*ast.Ident)
					if !ok || name.Name == "_" {
						continue
					}
					if containsToolchainGo(rhs, toolchainAliases, goTools) {
						goTools[name.Name] = true
					}
					if containsTestArgument(rhs, testArgs) {
						testArgs[name.Name] = true
					}
					if call, ok := rhs.(*ast.CallExpr); ok && isExecCommand(call, execAliases) {
						commandNames[call] = name.Name
					}
				}
			case *ast.ValueSpec:
				for index, rhs := range value.Values {
					if index >= len(value.Names) {
						break
					}
					name := value.Names[index].Name
					if containsToolchainGo(rhs, toolchainAliases, goTools) {
						goTools[name] = true
					}
					if containsTestArgument(rhs, testArgs) {
						testArgs[name] = true
					}
					if call, ok := rhs.(*ast.CallExpr); ok && isExecCommand(call, execAliases) {
						commandNames[call] = name
					}
				}
			case *ast.CallExpr:
				if isExecCommand(value, execAliases) {
					commandCalls = append(commandCalls, value)
				}
				if isInheritanceMarker(value, toolchainAliases) && len(value.Args) >= 2 {
					if name, ok := value.Args[1].(*ast.Ident); ok {
						markedNames[name.Name] = true
					}
				}
			}
			return true
		})
		for _, call := range commandCalls {
			if !commandRunsGoTest(call, toolchainAliases, execAliases, goTools, testArgs) {
				continue
			}
			name, assigned := commandNames[call]
			if assigned && markedNames[name] {
				continue
			}
			if !assigned {
				name = "<unassigned>"
			}
			position := fset.Position(call.Pos())
			issues = append(issues, goTestCommandIssue{line: position.Line, command: name})
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].line != issues[j].line {
			return issues[i].line < issues[j].line
		}
		return issues[i].command < issues[j].command
	})
	return issues
}

func importedPackageAliases(file *ast.File) (execAliases, toolchainAliases map[string]bool) {
	execAliases, toolchainAliases = map[string]bool{}, map[string]bool{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		alias := filepath.Base(path)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		switch {
		case path == "os/exec":
			execAliases[alias] = true
		case strings.HasSuffix(path, "/internal/testtoolchain"):
			toolchainAliases[alias] = true
		}
	}
	return execAliases, toolchainAliases
}

func containsToolchainGo(expression ast.Expr, aliases, variables map[string]bool) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			if selector, ok := value.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Go" {
				if packageName, ok := selector.X.(*ast.Ident); ok && aliases[packageName.Name] {
					found = true
				}
			}
		case *ast.Ident:
			if variables[value.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

func containsTestArgument(expression ast.Expr, variables map[string]bool) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.BasicLit:
			if value.Kind == token.STRING {
				literal, err := strconv.Unquote(value.Value)
				if err == nil && literal == "test" {
					found = true
				}
			}
		case *ast.Ident:
			if variables[value.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

func isExecCommand(call *ast.CallExpr, aliases map[string]bool) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (selector.Sel.Name != "Command" && selector.Sel.Name != "CommandContext") {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && aliases[packageName.Name]
}

func isInheritanceMarker(call *ast.CallExpr, aliases map[string]bool) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "InheritHostGOROOTLock" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && aliases[packageName.Name]
}

func commandRunsGoTest(call *ast.CallExpr, toolchainAliases, execAliases, goTools, testArgs map[string]bool) bool {
	if !isExecCommand(call, execAliases) {
		return false
	}
	hasGoTool, hasTestArgument := false, false
	for _, argument := range call.Args {
		if containsToolchainGo(argument, toolchainAliases, goTools) {
			hasGoTool = true
		}
		if containsTestArgument(argument, testArgs) {
			hasTestArgument = true
		}
	}
	return hasGoTool && hasTestArgument
}
