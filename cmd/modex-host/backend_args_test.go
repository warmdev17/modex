package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"reflect"
	"testing"
)

func TestServeArgsKeepsCodexContract(t *testing.T) {
	got := serveArgs("/work", backendSpec{kind: "codex", codex: "codex-bin", home: "/home/user/.codex"}, "/tmp/native.sock")
	want := []string{"--serve", "--workspace", "/work", "--codex", "codex-bin", "--native-socket", "/tmp/native.sock", "--codex-home", "/home/user/.codex"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
	got = serveArgs("/work", backendSpec{kind: "codex", codex: "codex"}, "/tmp/native.sock")
	want = []string{"--serve", "--workspace", "/work", "--codex", "codex", "--native-socket", "/tmp/native.sock"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
}

func TestSuperviseArgsKeepsCodexContract(t *testing.T) {
	got := superviseArgs("/work", backendSpec{kind: "codex", codex: "codex-bin", home: "/home/user/.codex"}, "/state")
	want := []string{"--supervise", "--workspace", "/work", "--codex", "codex-bin", "--state-dir", "/state", "--codex-home", "/home/user/.codex"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
	got = superviseArgs("/work", backendSpec{kind: "codex", codex: "codex"}, "/state")
	want = []string{"--supervise", "--workspace", "/work", "--codex", "codex", "--state-dir", "/state"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
}

func TestSuperviseArgsOpenCodeHasNoCodexHome(t *testing.T) {
	got := superviseArgs("/work", backendSpec{kind: "opencode", opencode: "opencode-bin", home: "/ignored"}, "/state")
	want := []string{"--supervise", "--workspace", "/work", "--backend", "opencode", "--opencode", "opencode-bin", "--state-dir", "/state"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
}

func TestServeArgsOpenCodeHasNoNativeSocket(t *testing.T) {
	got := serveArgs("/work", backendSpec{kind: "opencode", opencode: "opencode-bin", home: "/ignored"}, "/tmp/native.sock")
	want := []string{"--serve", "--workspace", "/work", "--backend", "opencode", "--opencode", "opencode-bin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
}

// TestHandlersUseCommandBuilders guards the call-site wiring itself: a mutant
// that inlines exec.Command/execCommand in superviseHost or startBackground
// fails even though the helper unit tests still pass.
func TestHandlersUseCommandBuilders(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "supervisor.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]string{
		"superviseHost":   "serveChildCommand",
		"startBackground": "superviseChildCommand",
	}
	seen := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		want, ok := wanted[fn.Name.Name]
		if !ok {
			continue
		}
		seen[fn.Name.Name] = true
		helper := false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				if fun.Name == "execCommand" {
					t.Errorf("%s calls execCommand directly instead of %s", fn.Name.Name, want)
				}
				if fun.Name == want {
					helper = true
				}
			case *ast.SelectorExpr:
				if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "exec" && fun.Sel.Name == "Command" {
					t.Errorf("%s calls exec.Command directly instead of %s", fn.Name.Name, want)
				}
			}
			return true
		})
		if !helper {
			t.Errorf("%s does not call %s", fn.Name.Name, want)
		}
	}
	for name := range wanted {
		if !seen[name] {
			t.Errorf("handler %s not found", name)
		}
	}
}

// TestModeCommandsUseArgHelpers guards the exec seam: a mode handler must pass
// the shared argv helper's output, not an inlined copy that could drift.
func TestModeCommandsUseArgHelpers(t *testing.T) {
	original := execCommand
	t.Cleanup(func() { execCommand = original })

	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		return exec.Command("true")
	}

	spec := backendSpec{kind: "codex", codex: "codex-bin", home: "/home/user/.codex"}
	_ = serveChildCommand("/self", "/work", spec, "/tmp/native.sock")
	_ = superviseChildCommand("/self", "/work", spec, "/state")

	wantServe := append([]string{"/self"}, serveArgs("/work", spec, "/tmp/native.sock")...)
	wantSupervise := append([]string{"/self"}, superviseArgs("/work", spec, "/state")...)
	if len(calls) != 2 || !reflect.DeepEqual(calls[0], wantServe) || !reflect.DeepEqual(calls[1], wantSupervise) {
		t.Fatalf("commands=%q want %q and %q", calls, wantServe, wantSupervise)
	}
}

func TestCodexSupervisorForwardsCustomOpenCodeBinary(t *testing.T) {
	spec := backendSpec{kind: "codex", codex: "codex", opencode: "/opt/opencode"}
	for _, args := range [][]string{serveArgs("/work", spec, "/tmp/socket"), superviseArgs("/work", spec, "/state")} {
		found := false
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "--opencode" && args[i+1] == "/opt/opencode" {
				found = true
			}
		}
		if !found {
			t.Fatalf("custom opencode missing: %q", args)
		}
	}
}
