package gittools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerNetwork(s *server.MCPServer, repo string) {
	s.AddTool(mcp.NewTool("git_fetch",
		mcp.WithDescription("Fetch updates from a remote."),
		mcp.WithString("remote", mcp.Description("Remote name (default origin)."), mcp.DefaultString("origin")),
		mcp.WithBoolean("prune", mcp.Description("Prune deleted remote-tracking branches.")),
		mcp.WithBoolean("all", mcp.Description("Fetch from all remotes.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
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
		mcp.WithString("remote", mcp.Description("Remote name (default origin)."), mcp.DefaultString("origin")),
		mcp.WithString("branch", mcp.Description("Remote branch (default: tracked upstream).")),
		mcp.WithBoolean("rebase", mcp.Description("Use --rebase instead of merge.")),
		mcp.WithBoolean("ff_only", mcp.Description("Only allow fast-forward updates.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
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
		mcp.WithString("remote", mcp.Description("Remote name (default origin)."), mcp.DefaultString("origin")),
		mcp.WithString("branch", mcp.Description("Local branch to push (default: current).")),
		mcp.WithBoolean("set_upstream", mcp.Description("Set upstream tracking (-u).")),
		mcp.WithBoolean("tags", mcp.Description("Push tags as well.")),
		mcp.WithBoolean("force", mcp.Description("Force push (uses --force-with-lease). Requires confirm=true.")),
		mcp.WithBoolean("confirm", mcp.Description("Required when force=true.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
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
