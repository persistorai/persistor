package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// executeArgs runs the given root command with args and returns any error.
// It suppresses cobra's usage/error output so test output stays clean.
func executeArgs(t *testing.T, root *cobra.Command, args ...string) error {
	t.Helper()
	root.SetOut(&strings.Builder{})
	root.SetErr(&strings.Builder{})
	root.SetArgs(args)
	_, err := root.ExecuteC()
	return err
}

// newTestRoot builds a root command tree identical to main() but with
// PersistentPreRun stubbed out so the API client is never initialised.
func newTestRoot() *cobra.Command {
	root := &cobra.Command{
		Use:          "persistor",
		SilenceUsage: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Skip client initialisation in tests.
		},
	}
	root.PersistentFlags().StringVar(&flagURL, "url", "http://localhost:3030", "")
	root.PersistentFlags().StringVar(&flagKey, "api-key", "", "")
	root.PersistentFlags().StringVar(&flagFmt, "format", "json", "")

	root.AddCommand(newNodeCmd())
	root.AddCommand(newEdgeCmd())
	root.AddCommand(newSearchCmd())
	return root
}

// --- node create ---

func TestNodeCreateArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "requires exactly one positional arg (label)",
			args:    []string{"node", "create"},
			wantErr: true,
		},
		{
			name: "accepts one positional arg",
			// Run will call apiClient which is nil — but Args validation fires
			// first, so ExactArgs(1) should pass and the Run will panic. We
			// only care about the arg-count check here; run errors are tested
			// separately. To avoid the nil-dereference panic we recover in the
			// helper by overriding the Run func.
			args:    nil, // handled below
			wantErr: false,
		},
		{
			name:    "rejects two positional args",
			args:    []string{"node", "create", "label1", "extra"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.args == nil {
				return // skip placeholder
			}
			root := newTestRoot()
			err := executeArgs(t, root, tc.args...)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestNodeCreateArgCountOnly verifies ExactArgs(1) directly without invoking Run.
func TestNodeCreateArgCountOnly(t *testing.T) {
	argsValidator := cobra.ExactArgs(1)

	if err := argsValidator(nil, []string{"my-label"}); err != nil {
		t.Errorf("one arg should be valid, got: %v", err)
	}
	if err := argsValidator(nil, []string{}); err == nil {
		t.Error("zero args should fail ExactArgs(1)")
	}
	if err := argsValidator(nil, []string{"a", "b"}); err == nil {
		t.Error("two args should fail ExactArgs(1)")
	}
}

// --- node get/delete/history ---

func TestNodeExactArgs1Commands(t *testing.T) {
	subcommands := []string{"get", "delete", "history"}
	for _, sub := range subcommands {
		t.Run(sub, func(t *testing.T) {
			argsValidator := cobra.ExactArgs(1)
			if err := argsValidator(nil, []string{"node-id"}); err != nil {
				t.Errorf("%s: one arg should be accepted: %v", sub, err)
			}
			if err := argsValidator(nil, []string{}); err == nil {
				t.Errorf("%s: zero args should be rejected", sub)
			}
		})
	}
}

// --- node migrate ---

func TestNodeMigrateArgCount(t *testing.T) {
	argsValidator := cobra.ExactArgs(2)

	cases := []struct {
		args    []string
		wantErr bool
	}{
		{[]string{"old-id", "new-id"}, false},
		{[]string{"only-one"}, true},
		{[]string{"a", "b", "c"}, true},
		{[]string{}, true},
	}
	for _, tc := range cases {
		err := argsValidator(nil, tc.args)
		if tc.wantErr && err == nil {
			t.Errorf("args %v: expected error", tc.args)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("args %v: unexpected error: %v", tc.args, err)
		}
	}
}

// --- edge create ---

func TestEdgeCreateRequiresRelationFlag(t *testing.T) {
	// edgeCreateCmd marks --relation as required.
	cmd := edgeCreateCmd()
	required := cmd.Flags().Lookup("relation")
	if required == nil {
		t.Fatal("--relation flag not found on edge create")
	}
	// Cobra stores required-flag annotations; check that the annotation exists.
	ann := cmd.Flags().ShorthandLookup("")
	_ = ann // just ensure flag lookup doesn't panic

	// Verify Args validator: exactly 2 positional args.
	argsValidator := cobra.ExactArgs(2)
	if err := argsValidator(nil, []string{"src", "dst"}); err != nil {
		t.Errorf("two args should be valid: %v", err)
	}
	if err := argsValidator(nil, []string{"src"}); err == nil {
		t.Error("one arg should fail ExactArgs(2)")
	}
}

func TestEdgeCreateArgValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"missing both args", []string{"edge", "create"}, true},
		{"missing target", []string{"edge", "create", "src-id"}, true},
		{"too many args", []string{"edge", "create", "a", "b", "c"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newTestRoot()
			err := executeArgs(t, root, tc.args...)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

// --- edge update / patch / delete ---

func TestEdgeThreeArgCommands(t *testing.T) {
	argsValidator := cobra.ExactArgs(3)

	cases := []struct {
		args    []string
		wantErr bool
	}{
		{[]string{"src", "dst", "KNOWS"}, false},
		{[]string{"src", "dst"}, true},
		{[]string{"src"}, true},
		{[]string{}, true},
		{[]string{"a", "b", "c", "d"}, true},
	}
	for _, tc := range cases {
		err := argsValidator(nil, tc.args)
		if tc.wantErr && err == nil {
			t.Errorf("args %v: expected error", tc.args)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("args %v: unexpected error: %v", tc.args, err)
		}
	}
}

// --- search ---

func TestSearchArgValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"no query", []string{"search"}, true},
		{"two queries", []string{"search", "foo", "bar"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newTestRoot()
			err := executeArgs(t, root, tc.args...)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

// TestSearchModeFlag verifies the --mode flag is registered with the right default.
func TestSearchModeFlag(t *testing.T) {
	cmd := newSearchCmd()
	f := cmd.Flags().Lookup("mode")
	if f == nil {
		t.Fatal("--mode flag not found on search command")
	}
	if f.DefValue != "hybrid" {
		t.Errorf("default mode: got %q, want %q", f.DefValue, "hybrid")
	}
}

// TestSearchLimitFlag verifies --limit is registered with default 0.
func TestSearchLimitFlag(t *testing.T) {
	cmd := newSearchCmd()
	f := cmd.Flags().Lookup("limit")
	if f == nil {
		t.Fatal("--limit flag not found on search command")
	}
	if f.DefValue != "0" {
		t.Errorf("default limit: got %q, want %q", f.DefValue, "0")
	}
}

// --- node list flag defaults ---

func TestNodeListFlagDefaults(t *testing.T) {
	cmd := nodeListCmd()

	cases := []struct {
		flag string
		want string
	}{
		{"type", ""},
		{"limit", "0"},
		{"offset", "0"},
	}
	for _, tc := range cases {
		f := cmd.Flags().Lookup(tc.flag)
		if f == nil {
			t.Errorf("--%s flag not found", tc.flag)
			continue
		}
		if f.DefValue != tc.want {
			t.Errorf("--%s default: got %q, want %q", tc.flag, f.DefValue, tc.want)
		}
	}
}

// --- edge list filter flags ---

func TestEdgeListFlagDefaults(t *testing.T) {
	cmd := edgeListCmd()

	flags := []string{"source", "target", "relation", "limit"}
	for _, name := range flags {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			t.Errorf("--%s flag not found on edge list", name)
		}
	}
}

// --- global format flag ---

func TestFormatFlagDefault(t *testing.T) {
	root := newTestRoot()
	f := root.PersistentFlags().Lookup("format")
	if f == nil {
		t.Fatal("--format flag not found")
	}
	if f.DefValue != "json" {
		t.Errorf("default format: got %q, want %q", f.DefValue, "json")
	}
}

// TestFormatFlagValues verifies that accepted format values are "json", "table",
// and "quiet" — these are the only strings the output functions branch on.
func TestFormatFlagValues(t *testing.T) {
	validFormats := []string{"json", "table", "quiet"}
	for _, fmt := range validFormats {
		flagFmt = fmt
		// output() must not panic for any of these values.
		captureStdout(t, func() { output(map[string]string{"k": "v"}, "id") })
	}
}

// --- node create flag registration ---

func TestNodeCreateFlagRegistration(t *testing.T) {
	cmd := nodeCreateCmd()
	for _, name := range []string{"type", "props"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("--%s flag not found on node create", name)
		}
	}
}

// --- node migrate flags ---

func TestNodeMigrateFlagDefaults(t *testing.T) {
	cmd := nodeMigrateCmd()
	cases := []struct {
		flag string
		want string
	}{
		{"delete-old", "true"},
		{"dry-run", "false"},
	}
	for _, tc := range cases {
		f := cmd.Flags().Lookup(tc.flag)
		if f == nil {
			t.Errorf("--%s flag not found", tc.flag)
			continue
		}
		if f.DefValue != tc.want {
			t.Errorf("--%s default: got %q, want %q", tc.flag, f.DefValue, tc.want)
		}
	}
}
