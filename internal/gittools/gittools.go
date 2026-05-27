// Package gittools registers git-related MCP tools on a server.
package gittools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ResolveRepo validates that path points to a git repository and returns its
// absolute path.
func ResolveRepo(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve repo path: %w", err)
	}
	if _, err := git.PlainOpenWithOptions(abs, &git.PlainOpenOptions{DetectDotGit: true}); err != nil {
		return "", fmt.Errorf("open repo at %s: %w", abs, err)
	}
	return abs, nil
}

// openRepo opens the repository, walking upward to find the .git directory.
func openRepo(path string) (*git.Repository, error) {
	return git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
}

// Register attaches every git tool to the MCP server. defaultRepo is used for
// any tool call that omits the per-call "repo" argument; it may be empty, in
// which case a "repo" argument becomes mandatory.
func Register(s *server.MCPServer, defaultRepo string) {
	registerRead(s, defaultRepo)
	registerWrite(s, defaultRepo)
	registerNetwork(s, defaultRepo)
	registerDangerous(s, defaultRepo)
	registerWorktree(s, defaultRepo)
	registerAdvanced(s, defaultRepo)
}

// repoArgDesc documents the optional per-call repository argument.
const repoArgDesc = "Path to the git repository to operate on (absolute, or relative to the server's working directory). " +
	"Defaults to the server's startup repository when omitted."

// withRepo adds the optional "repo" argument shared by every git tool, letting
// the caller choose which repository the operation targets.
func withRepo() mcp.ToolOption {
	return mcp.WithString("repo", mcp.Description(repoArgDesc))
}

// repoDir resolves the repository directory for a single tool call. A non-empty
// "repo" argument is validated and returned as an absolute path; otherwise the
// server's default repository is used. If neither is available, it errors.
func (a args) repoDir(def string) (string, error) {
	p := a.String("repo", "")
	if p == "" {
		if def == "" {
			return "", fmt.Errorf("no repository selected: pass a \"repo\" argument, or start git-mcp with -repo / GIT_MCP_REPO")
		}
		return def, nil
	}
	// Reject UNC / network paths: opening one on Windows triggers NTLM auth to
	// the remote host, leaking the user's credential hash. A caller-supplied
	// repo must be a local filesystem path.
	if isNetworkPath(p) {
		return "", fmt.Errorf("repo %q is a network/UNC path; only local repositories are allowed", p)
	}
	return ResolveRepo(p)
}

// isNetworkPath reports whether p refers to a UNC / network location.
func isNetworkPath(p string) bool {
	if strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//") {
		return true
	}
	vol := filepath.VolumeName(p)
	return strings.HasPrefix(vol, `\\`) || strings.HasPrefix(vol, "//")
}

// runGit shells out to the system `git` binary in the given repo directory.
// Used for operations go-git does not support (worktree, rebase, cherry-pick,
// stash push/pop, clean) or where auth handling is simpler via the CLI
// (fetch/pull/push).
func runGit(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := strings.TrimRight(stdout.String(), "\n")
	if err != nil {
		errOut := strings.TrimRight(stderr.String(), "\n")
		return out, fmt.Errorf("git %s failed: %v\n%s", strings.Join(args, " "), err, errOut)
	}
	if out == "" {
		out = strings.TrimRight(stderr.String(), "\n")
	}
	return out, nil
}

// textResult is shorthand for returning a plain-text CallToolResult.
func textResult(s string) *mcp.CallToolResult {
	if s == "" {
		s = "(no output)"
	}
	return mcp.NewToolResultText(s)
}

// errResult wraps an error as a tool error result.
func errResult(err error) *mcp.CallToolResult {
	return mcp.NewToolResultError(err.Error())
}

// args wraps the raw argument map with typed getters.
type args map[string]any

func argsOf(req mcp.CallToolRequest) args {
	if req.Params.Arguments == nil {
		return args{}
	}
	return args(req.Params.Arguments)
}

func (a args) String(key, def string) string {
	v, ok := a[key]
	if !ok || v == nil {
		return def
	}
	s, ok := v.(string)
	if !ok {
		return def
	}
	return s
}

func (a args) RequireString(key string) (string, error) {
	v, ok := a[key]
	if !ok || v == nil {
		return "", fmt.Errorf("missing required argument %q", key)
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("argument %q must be a non-empty string", key)
	}
	return s, nil
}

// safeRef rejects values that would be interpreted as git command-line flags.
// Callers must run this on every user-supplied ref / branch / remote / revision
// before appending it to a git argv.
func safeRef(name, value string) error {
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("argument %q must not start with '-' (would be parsed as a git option): %q", name, value)
	}
	return nil
}

func (a args) Bool(key string, def bool) bool {
	v, ok := a[key]
	if !ok || v == nil {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

func (a args) Int(key string, def int) int {
	v, ok := a[key]
	if !ok || v == nil {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return def
}

func (a args) StringSlice(key string) ([]string, error) {
	v, ok := a[key]
	if !ok || v == nil {
		return nil, fmt.Errorf("missing required argument %q", key)
	}
	raw, ok := v.([]any)
	if !ok {
		// Some clients may send a typed slice directly.
		if ss, ok := v.([]string); ok {
			return ss, nil
		}
		return nil, fmt.Errorf("argument %q must be an array", key)
	}
	out := make([]string, 0, len(raw))
	for i, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("argument %q[%d] must be a string", key, i)
		}
		out = append(out, s)
	}
	return out, nil
}
