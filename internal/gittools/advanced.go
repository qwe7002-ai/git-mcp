package gittools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerAdvanced(s *server.MCPServer, defaultRepo string) {
	s.AddTool(mcp.NewTool("git_rebase",
		mcp.WithDescription("Rebase the current branch onto another. Interactive rebase is not supported."),
		withRepo(),
		mcp.WithString("onto", mcp.Required(), mcp.Description("Target branch or revision to rebase onto.")),
		mcp.WithBoolean("abort", mcp.Description("Abort an in-progress rebase (ignores 'onto').")),
		mcp.WithBoolean("continue", mcp.Description("Continue an in-progress rebase after resolving conflicts.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		if a.Bool("abort", false) {
			out, err := runGit(ctx, repo, "rebase", "--abort")
			if err != nil {
				return errResult(err), nil
			}
			return textResult(out), nil
		}
		if a.Bool("continue", false) {
			out, err := runGit(ctx, repo, "rebase", "--continue")
			if err != nil {
				return errResult(err), nil
			}
			return textResult(out), nil
		}
		onto, err := a.RequireString("onto")
		if err != nil {
			return errResult(err), nil
		}
		if err := safeRef("onto", onto); err != nil {
			return errResult(err), nil
		}
		out, err := runGit(ctx, repo, "rebase", onto)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_cherry_pick",
		mcp.WithDescription("Apply commits onto the current branch."),
		withRepo(),
		mcp.WithArray("commits", mcp.Description("List of commit revisions to cherry-pick."),
			mcp.Items(map[string]any{"type": "string"})),
		mcp.WithBoolean("abort", mcp.Description("Abort an in-progress cherry-pick.")),
		mcp.WithBoolean("continue", mcp.Description("Continue an in-progress cherry-pick.")),
		mcp.WithBoolean("no_commit", mcp.Description("Apply changes without committing (-n).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		if a.Bool("abort", false) {
			out, err := runGit(ctx, repo, "cherry-pick", "--abort")
			if err != nil {
				return errResult(err), nil
			}
			return textResult(out), nil
		}
		if a.Bool("continue", false) {
			out, err := runGit(ctx, repo, "cherry-pick", "--continue")
			if err != nil {
				return errResult(err), nil
			}
			return textResult(out), nil
		}
		commits, err := a.StringSlice("commits")
		if err != nil {
			return errResult(err), nil
		}
		if len(commits) == 0 {
			return errResult(fmt.Errorf("commits must not be empty")), nil
		}
		for i, c := range commits {
			if err := safeRef(fmt.Sprintf("commits[%d]", i), c); err != nil {
				return errResult(err), nil
			}
		}
		args := []string{"cherry-pick"}
		if a.Bool("no_commit", false) {
			args = append(args, "-n")
		}
		args = append(args, commits...)
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})
}
