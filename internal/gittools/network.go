package gittools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// safeURL rejects URLs that would be parsed as a git option or contain
// whitespace / NUL. We deliberately do not constrain the scheme — callers may
// legitimately use https://, ssh://, git://, file://, or scp-style host:path.
func safeURL(name, value string) error {
	if value == "" {
		return fmt.Errorf("argument %q must not be empty", name)
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("argument %q must not start with '-' (would be parsed as a git option): %q", name, value)
	}
	if i := strings.IndexAny(value, " \t\r\n\x00"); i >= 0 {
		return fmt.Errorf("argument %q must not contain whitespace: %q", name, value)
	}
	return nil
}

// resolveTargetDir validates a caller-supplied filesystem destination for
// clone/init. It rejects empty values, leading '-', and UNC/network paths
// (same rationale as repoDir), and returns an absolute path.
func resolveTargetDir(name, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("argument %q must not be empty", name)
	}
	if err := safePath(name, value); err != nil {
		return "", err
	}
	if isNetworkPath(value) {
		return "", fmt.Errorf("%s %q is a network/UNC path; only local destinations are allowed", name, value)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", name, err)
	}
	return abs, nil
}

func registerNetwork(s *server.MCPServer, defaultRepo string) {
	s.AddTool(mcp.NewTool("git_clone",
		mcp.WithDescription("Clone a remote repository into a local directory. The target directory must not exist (or must be empty). Does not require an existing repo context."),
		mcp.WithString("url", mcp.Required(), mcp.Description("Repository URL (https://, ssh://, git://, file://, or scp-style host:path).")),
		mcp.WithString("directory", mcp.Required(), mcp.Description("Local target directory (absolute, or relative to the server's working directory). UNC paths are rejected.")),
		mcp.WithString("branch", mcp.Description("Branch or tag to check out after clone (-b).")),
		mcp.WithNumber("depth", mcp.Description("Create a shallow clone with the given depth (--depth N).")),
		mcp.WithBoolean("recurse_submodules", mcp.Description("Also clone submodules recursively.")),
		mcp.WithBoolean("bare", mcp.Description("Create a bare repository (--bare).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		url, err := a.RequireString("url")
		if err != nil {
			return errResult(err), nil
		}
		if err := safeURL("url", url); err != nil {
			return errResult(err), nil
		}
		dir, err := a.RequireString("directory")
		if err != nil {
			return errResult(err), nil
		}
		target, err := resolveTargetDir("directory", dir)
		if err != nil {
			return errResult(err), nil
		}
		// Reject pre-existing non-empty targets so we don't accidentally clone
		// into a directory with the user's other work in it.
		if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
			return errResult(fmt.Errorf("target directory %q is not empty", target)), nil
		}
		parent := filepath.Dir(target)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return errResult(fmt.Errorf("create parent directory: %w", err)), nil
		}
		args := []string{"clone"}
		if b := a.String("branch", ""); b != "" {
			if err := safeRef("branch", b); err != nil {
				return errResult(err), nil
			}
			args = append(args, "-b", b)
		}
		if d := a.Int("depth", 0); d > 0 {
			args = append(args, fmt.Sprintf("--depth=%d", d))
		}
		if a.Bool("recurse_submodules", false) {
			args = append(args, "--recurse-submodules")
		}
		if a.Bool("bare", false) {
			args = append(args, "--bare")
		}
		args = append(args, "--", url, target)
		out, err := runGit(ctx, parent, args...)
		if err != nil {
			return errResult(err), nil
		}
		if out == "" {
			out = fmt.Sprintf("cloned %s into %s", url, target)
		} else {
			out = fmt.Sprintf("%s\ncloned into %s", out, target)
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_init",
		mcp.WithDescription("Initialize a new git repository at the given directory. Creates the directory if it does not exist."),
		mcp.WithString("directory", mcp.Required(), mcp.Description("Target directory for the new repo (absolute, or relative to the server's working directory). UNC paths are rejected.")),
		mcp.WithBoolean("bare", mcp.Description("Create a bare repository (--bare).")),
		mcp.WithString("initial_branch", mcp.Description("Initial branch name (default: git's configured init.defaultBranch).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		dir, err := a.RequireString("directory")
		if err != nil {
			return errResult(err), nil
		}
		target, err := resolveTargetDir("directory", dir)
		if err != nil {
			return errResult(err), nil
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return errResult(fmt.Errorf("create target directory: %w", err)), nil
		}
		args := []string{"init"}
		if a.Bool("bare", false) {
			args = append(args, "--bare")
		}
		if b := a.String("initial_branch", ""); b != "" {
			if err := safeRef("initial_branch", b); err != nil {
				return errResult(err), nil
			}
			args = append(args, "--initial-branch="+b)
		}
		args = append(args, target)
		out, err := runGit(ctx, target, args...)
		if err != nil {
			return errResult(err), nil
		}
		if out == "" {
			out = fmt.Sprintf("initialized repository at %s", target)
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_remote_add",
		mcp.WithDescription("Add a new remote to the repository."),
		withRepo(),
		mcp.WithString("name", mcp.Required(), mcp.Description("Remote name (e.g. origin, upstream).")),
		mcp.WithString("url", mcp.Required(), mcp.Description("Remote URL.")),
		mcp.WithBoolean("fetch", mcp.Description("Run an immediate fetch from the new remote (-f).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		name, err := a.RequireString("name")
		if err != nil {
			return errResult(err), nil
		}
		if err := safeRef("name", name); err != nil {
			return errResult(err), nil
		}
		url, err := a.RequireString("url")
		if err != nil {
			return errResult(err), nil
		}
		if err := safeURL("url", url); err != nil {
			return errResult(err), nil
		}
		args := []string{"remote", "add"}
		if a.Bool("fetch", false) {
			args = append(args, "-f")
		}
		args = append(args, name, url)
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		if out == "" {
			out = fmt.Sprintf("added remote %s -> %s", name, url)
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_remote_remove",
		mcp.WithDescription("Remove a remote from the repository."),
		withRepo(),
		mcp.WithString("name", mcp.Required(), mcp.Description("Remote name to remove.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		name, err := a.RequireString("name")
		if err != nil {
			return errResult(err), nil
		}
		if err := safeRef("name", name); err != nil {
			return errResult(err), nil
		}
		out, err := runGit(ctx, repo, "remote", "remove", name)
		if err != nil {
			return errResult(err), nil
		}
		if out == "" {
			out = fmt.Sprintf("removed remote %s", name)
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_remote_set_url",
		mcp.WithDescription("Change the URL of an existing remote."),
		withRepo(),
		mcp.WithString("name", mcp.Required(), mcp.Description("Remote name.")),
		mcp.WithString("url", mcp.Required(), mcp.Description("New remote URL.")),
		mcp.WithBoolean("push", mcp.Description("Set the push URL instead of the fetch URL (--push).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		name, err := a.RequireString("name")
		if err != nil {
			return errResult(err), nil
		}
		if err := safeRef("name", name); err != nil {
			return errResult(err), nil
		}
		url, err := a.RequireString("url")
		if err != nil {
			return errResult(err), nil
		}
		if err := safeURL("url", url); err != nil {
			return errResult(err), nil
		}
		args := []string{"remote", "set-url"}
		if a.Bool("push", false) {
			args = append(args, "--push")
		}
		args = append(args, name, url)
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		if out == "" {
			out = fmt.Sprintf("set remote %s url -> %s", name, url)
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_fetch",
		mcp.WithDescription("Fetch updates from a remote."),
		withRepo(),
		mcp.WithString("remote", mcp.Description("Remote name (default origin)."), mcp.DefaultString("origin")),
		mcp.WithBoolean("prune", mcp.Description("Prune deleted remote-tracking branches.")),
		mcp.WithBoolean("all", mcp.Description("Fetch from all remotes.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		args := []string{"fetch"}
		if a.Bool("prune", false) {
			args = append(args, "--prune")
		}
		if a.Bool("all", false) {
			args = append(args, "--all")
		} else {
			remote := a.String("remote", "origin")
			if err := safeRef("remote", remote); err != nil {
				return errResult(err), nil
			}
			args = append(args, remote)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_pull",
		mcp.WithDescription("Pull from a remote into the current branch."),
		withRepo(),
		mcp.WithString("remote", mcp.Description("Remote name (default origin)."), mcp.DefaultString("origin")),
		mcp.WithString("branch", mcp.Description("Remote branch (default: tracked upstream).")),
		mcp.WithBoolean("rebase", mcp.Description("Use --rebase instead of merge.")),
		mcp.WithBoolean("ff_only", mcp.Description("Only allow fast-forward updates.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		args := []string{"pull"}
		if a.Bool("rebase", false) {
			args = append(args, "--rebase")
		}
		if a.Bool("ff_only", false) {
			args = append(args, "--ff-only")
		}
		remote := a.String("remote", "origin")
		if err := safeRef("remote", remote); err != nil {
			return errResult(err), nil
		}
		args = append(args, remote)
		if b := a.String("branch", ""); b != "" {
			if err := safeRef("branch", b); err != nil {
				return errResult(err), nil
			}
			args = append(args, b)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_push",
		mcp.WithDescription("Push commits to a remote. force=true requires confirm=true (rewrites remote history)."),
		withRepo(),
		mcp.WithString("remote", mcp.Description("Remote name (default origin)."), mcp.DefaultString("origin")),
		mcp.WithString("branch", mcp.Description("Local branch to push (default: current).")),
		mcp.WithBoolean("set_upstream", mcp.Description("Set upstream tracking (-u).")),
		mcp.WithBoolean("tags", mcp.Description("Push tags as well.")),
		mcp.WithBoolean("force", mcp.Description("Force push (uses --force-with-lease). Requires confirm=true.")),
		mcp.WithBoolean("confirm", mcp.Description("Required when force=true.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		force := a.Bool("force", false)
		if force && !a.Bool("confirm", false) {
			return errResult(fmt.Errorf("force push rewrites remote history; pass confirm=true to proceed")), nil
		}
		args := []string{"push"}
		if a.Bool("set_upstream", false) {
			args = append(args, "-u")
		}
		if a.Bool("tags", false) {
			args = append(args, "--tags")
		}
		if force {
			args = append(args, "--force-with-lease")
		}
		remote := a.String("remote", "origin")
		if err := safeRef("remote", remote); err != nil {
			return errResult(err), nil
		}
		args = append(args, remote)
		if b := a.String("branch", ""); b != "" {
			if err := safeRef("branch", b); err != nil {
				return errResult(err), nil
			}
			args = append(args, b)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})
}
