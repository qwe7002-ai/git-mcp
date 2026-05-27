package gittools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerDangerous(s *server.MCPServer, defaultRepo string) {
	s.AddTool(mcp.NewTool("git_clean",
		mcp.WithDescription("Remove untracked files (and optionally directories). Destructive — requires confirm=true."),
		withRepo(),
		mcp.WithBoolean("directories", mcp.Description("Also remove untracked directories (-d).")),
		mcp.WithBoolean("ignored", mcp.Description("Also remove ignored files (-x).")),
		mcp.WithBoolean("dry_run", mcp.Description("List what would be removed without deleting (overrides confirm).")),
		mcp.WithBoolean("confirm", mcp.Description("Required unless dry_run=true. Acknowledges deletion of untracked files.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		dry := a.Bool("dry_run", false)
		if !dry && !a.Bool("confirm", false) {
			return errResult(fmt.Errorf("git clean deletes untracked files; pass confirm=true or dry_run=true")), nil
		}
		args := []string{"clean", "-f"}
		if dry {
			args = []string{"clean", "-n"}
		}
		if a.Bool("directories", false) {
			args = append(args, "-d")
		}
		if a.Bool("ignored", false) {
			args = append(args, "-x")
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})
}
