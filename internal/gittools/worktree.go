package gittools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWorktree(s *server.MCPServer, defaultRepo string) {
	s.AddTool(mcp.NewTool("git_worktree_add",
		mcp.WithDescription("Create a new worktree at the given path, optionally checking out or creating a branch."),
		withRepo(),
		mcp.WithString("path", mcp.Required(), mcp.Description("Filesystem path for the new worktree.")),
		mcp.WithString("branch", mcp.Description("Branch to check out in the worktree.")),
		mcp.WithBoolean("create_branch", mcp.Description("Create a new branch named by 'branch' starting at 'from'.")),
		mcp.WithString("from", mcp.Description("Starting point when create_branch=true (default HEAD).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		path, err := a.RequireString("path")
		if err != nil {
			return errResult(err), nil
		}
		if err := safePath("path", path); err != nil {
			return errResult(err), nil
		}
		args := []string{"worktree", "add"}
		branch := a.String("branch", "")
		if branch != "" {
			if err := safeRef("branch", branch); err != nil {
				return errResult(err), nil
			}
		}
		if a.Bool("create_branch", false) {
			if branch == "" {
				return errResult(fmt.Errorf("branch is required when create_branch=true")), nil
			}
			args = append(args, "-b", branch, path)
			if from := a.String("from", ""); from != "" {
				if err := safeRef("from", from); err != nil {
					return errResult(err), nil
				}
				args = append(args, from)
			}
		} else {
			args = append(args, path)
			if branch != "" {
				args = append(args, branch)
			}
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_worktree_remove",
		mcp.WithDescription("Remove a linked worktree."),
		withRepo(),
		mcp.WithString("path", mcp.Required(), mcp.Description("Worktree path to remove.")),
		mcp.WithBoolean("force", mcp.Description("Force remove even with local modifications.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		path, err := a.RequireString("path")
		if err != nil {
			return errResult(err), nil
		}
		if err := safePath("path", path); err != nil {
			return errResult(err), nil
		}
		args := []string{"worktree", "remove"}
		if a.Bool("force", false) {
			args = append(args, "--force")
		}
		args = append(args, path)
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		if out == "" {
			out = fmt.Sprintf("removed worktree %s", path)
		}
		return textResult(out), nil
	})
}
