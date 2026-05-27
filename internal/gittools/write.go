package gittools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWrite(s *server.MCPServer, defaultRepo string) {
	s.AddTool(mcp.NewTool("git_add",
		mcp.WithDescription("Stage paths for the next commit. Use '.' for all changes."),
		withRepo(),
		mcp.WithArray("paths", mcp.Required(), mcp.Description("List of paths or patterns to stage."),
			mcp.Items(map[string]any{"type": "string"})),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		paths, err := a.StringSlice("paths")
		if err != nil {
			return errResult(err), nil
		}
		if len(paths) == 0 {
			return errResult(fmt.Errorf("paths must not be empty")), nil
		}
		args := append([]string{"add", "--"}, paths...)
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		if out == "" {
			out = fmt.Sprintf("staged: %v", paths)
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_commit",
		mcp.WithDescription("Create a commit. Set amend=true to amend HEAD; set all=true to auto-stage tracked changes."),
		withRepo(),
		mcp.WithString("message", mcp.Description("Commit message. Required unless amend=true with no message change.")),
		mcp.WithBoolean("amend", mcp.Description("Amend the previous commit.")),
		mcp.WithBoolean("all", mcp.Description("Stage modified/deleted tracked files automatically (git commit -a).")),
		mcp.WithBoolean("allow_empty", mcp.Description("Allow creating an empty commit.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		msg := a.String("message", "")
		amend := a.Bool("amend", false)
		all := a.Bool("all", false)
		allowEmpty := a.Bool("allow_empty", false)

		if msg == "" && !amend {
			return errResult(fmt.Errorf("message is required unless amend=true")), nil
		}
		args := []string{"commit"}
		if all {
			args = append(args, "-a")
		}
		if amend {
			args = append(args, "--amend")
			if msg == "" {
				args = append(args, "--no-edit")
			}
		}
		if allowEmpty {
			args = append(args, "--allow-empty")
		}
		if msg != "" {
			args = append(args, "-m", msg)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_checkout",
		mcp.WithDescription("Switch to an existing branch/ref, or create a new branch with create=true."),
		withRepo(),
		mcp.WithString("ref", mcp.Required(), mcp.Description("Branch name or revision to switch to.")),
		mcp.WithBoolean("create", mcp.Description("Create a new branch at the current HEAD (or 'from' if set).")),
		mcp.WithString("from", mcp.Description("Starting point when create=true.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		ref, err := a.RequireString("ref")
		if err != nil {
			return errResult(err), nil
		}
		if err := safeRef("ref", ref); err != nil {
			return errResult(err), nil
		}
		args := []string{"checkout"}
		if a.Bool("create", false) {
			args = append(args, "-b", ref)
			if from := a.String("from", ""); from != "" {
				if err := safeRef("from", from); err != nil {
					return errResult(err), nil
				}
				args = append(args, from)
			}
		} else {
			args = append(args, ref)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_branch_create",
		mcp.WithDescription("Create a new branch without switching to it."),
		withRepo(),
		mcp.WithString("name", mcp.Required(), mcp.Description("New branch name.")),
		mcp.WithString("from", mcp.Description("Starting point (default: HEAD).")),
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
		args := []string{"branch", name}
		if from := a.String("from", ""); from != "" {
			if err := safeRef("from", from); err != nil {
				return errResult(err), nil
			}
			args = append(args, from)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		if out == "" {
			out = fmt.Sprintf("created branch %s", name)
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_branch_delete",
		mcp.WithDescription("Delete a local branch. Use force=true to delete an unmerged branch."),
		withRepo(),
		mcp.WithString("name", mcp.Required(), mcp.Description("Branch name to delete.")),
		mcp.WithBoolean("force", mcp.Description("Force delete even if not merged.")),
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
		flag := "-d"
		if a.Bool("force", false) {
			flag = "-D"
		}
		out, err := runGit(ctx, repo, "branch", flag, name)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_merge",
		mcp.WithDescription("Merge a branch into the current branch."),
		withRepo(),
		mcp.WithString("branch", mcp.Required(), mcp.Description("Branch to merge from.")),
		mcp.WithBoolean("no_ff", mcp.Description("Create a merge commit even when fast-forward is possible.")),
		mcp.WithString("message", mcp.Description("Custom merge commit message.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		branch, err := a.RequireString("branch")
		if err != nil {
			return errResult(err), nil
		}
		if err := safeRef("branch", branch); err != nil {
			return errResult(err), nil
		}
		args := []string{"merge"}
		if a.Bool("no_ff", false) {
			args = append(args, "--no-ff")
		}
		if m := a.String("message", ""); m != "" {
			args = append(args, "-m", m)
		}
		args = append(args, branch)
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_reset",
		mcp.WithDescription("Reset the current HEAD to a revision. mode=hard requires confirm=true (destructive)."),
		withRepo(),
		mcp.WithString("mode", mcp.Description("soft | mixed | hard (default mixed)."), mcp.DefaultString("mixed"),
			mcp.Enum("soft", "mixed", "hard")),
		mcp.WithString("revision", mcp.Description("Revision to reset to (default HEAD)."), mcp.DefaultString("HEAD")),
		mcp.WithBoolean("confirm", mcp.Description("Required when mode=hard; acknowledges that uncommitted changes will be lost.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		mode := a.String("mode", "mixed")
		rev := a.String("revision", "HEAD")
		if err := safeRef("revision", rev); err != nil {
			return errResult(err), nil
		}
		if mode == "hard" && !a.Bool("confirm", false) {
			return errResult(fmt.Errorf("reset --hard is destructive; pass confirm=true to proceed")), nil
		}
		out, err := runGit(ctx, repo, "reset", "--"+mode, rev)
		if err != nil {
			return errResult(err), nil
		}
		if out == "" {
			out = fmt.Sprintf("reset (%s) to %s", mode, rev)
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_tag_create",
		mcp.WithDescription("Create a tag at the current HEAD or a given revision."),
		withRepo(),
		mcp.WithString("name", mcp.Required(), mcp.Description("Tag name.")),
		mcp.WithString("message", mcp.Description("Annotated tag message. If set, creates an annotated tag.")),
		mcp.WithString("revision", mcp.Description("Revision to tag (default HEAD).")),
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
		args := []string{"tag"}
		if m := a.String("message", ""); m != "" {
			args = append(args, "-a", name, "-m", m)
		} else {
			args = append(args, name)
		}
		if rev := a.String("revision", ""); rev != "" {
			if err := safeRef("revision", rev); err != nil {
				return errResult(err), nil
			}
			args = append(args, rev)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		if out == "" {
			out = fmt.Sprintf("created tag %s", name)
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_stash",
		mcp.WithDescription("Stash uncommitted changes."),
		withRepo(),
		mcp.WithString("message", mcp.Description("Optional stash message.")),
		mcp.WithBoolean("include_untracked", mcp.Description("Also stash untracked files.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		args := []string{"stash", "push"}
		if a.Bool("include_untracked", false) {
			args = append(args, "-u")
		}
		if m := a.String("message", ""); m != "" {
			args = append(args, "-m", m)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_stash_pop",
		mcp.WithDescription("Apply and remove the most recent (or specified) stash entry."),
		withRepo(),
		mcp.WithString("ref", mcp.Description("Stash ref like stash@{0}. Defaults to the most recent.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		args := []string{"stash", "pop"}
		if r := a.String("ref", ""); r != "" {
			if err := safeRef("ref", r); err != nil {
				return errResult(err), nil
			}
			args = append(args, r)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})
}
