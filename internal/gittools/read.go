package gittools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerRead(s *server.MCPServer, defaultRepo string) {
	s.AddTool(mcp.NewTool("git_status",
		mcp.WithDescription("Show the working tree status (porcelain format)."),
		withRepo(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := argsOf(req).repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		out, err := runGit(ctx, repo, "status", "--branch", "--short")
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_log",
		mcp.WithDescription("Show commit history. Optional limit (default 20) and revision range."),
		withRepo(),
		mcp.WithNumber("limit", mcp.Description("Maximum number of commits to return."), mcp.DefaultNumber(20)),
		mcp.WithString("revision", mcp.Description("Branch, tag, or revision range (e.g. main, HEAD~10..HEAD).")),
		mcp.WithString("path", mcp.Description("Restrict log to a specific path.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		limit := a.Int("limit", 20)
		if limit <= 0 {
			limit = 20
		}
		args := []string{"log", fmt.Sprintf("-n%d", limit), "--pretty=format:%h %ad %an %s", "--date=short"}
		if rev := a.String("revision", ""); rev != "" {
			if err := safeRef("revision", rev); err != nil {
				return errResult(err), nil
			}
			args = append(args, rev)
		}
		if p := a.String("path", ""); p != "" {
			args = append(args, "--", p)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_diff",
		mcp.WithDescription("Show diff for working tree or staged changes. Output is capped (~60KB by default) to fit LLM context; use summary/name_only or path to narrow large diffs."),
		withRepo(),
		mcp.WithBoolean("staged", mcp.Description("If true, show staged (index) diff instead of working tree.")),
		mcp.WithString("revision", mcp.Description("Diff against this revision instead of HEAD/index.")),
		mcp.WithString("path", mcp.Description("Restrict diff to a path.")),
		mcp.WithBoolean("summary", mcp.Description("Return only --stat (per-file +/- summary) instead of the full patch.")),
		mcp.WithBoolean("name_only", mcp.Description("Return only the list of changed file paths.")),
		mcp.WithNumber("max_bytes", mcp.Description("Cap output size in bytes (default 60000, 0 = unlimited). Output is truncated on a line boundary with a notice.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		args := []string{"diff", "--no-color"}
		switch {
		case a.Bool("name_only", false):
			args = append(args, "--name-only")
		case a.Bool("summary", false):
			args = append(args, "--stat")
		}
		if a.Bool("staged", false) {
			args = append(args, "--cached")
		}
		if rev := a.String("revision", ""); rev != "" {
			if err := safeRef("revision", rev); err != nil {
				return errResult(err), nil
			}
			args = append(args, rev)
		}
		if p := a.String("path", ""); p != "" {
			args = append(args, "--", p)
		}
		out, err := runGit(ctx, repo, args...)
		if err != nil {
			return errResult(err), nil
		}
		max := a.Int("max_bytes", defaultMaxBytes)
		out = truncate(out, max, "Re-run with summary=true, name_only=true, a path filter, or a larger max_bytes to see more.")
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_show",
		mcp.WithDescription("Show a commit, tag, or object. Output is capped (~60KB by default) to fit LLM context; use summary for large commits."),
		withRepo(),
		mcp.WithString("revision", mcp.Required(), mcp.Description("Revision to show (commit hash, tag, HEAD).")),
		mcp.WithBoolean("summary", mcp.Description("Show only the commit metadata and --stat instead of the full patch.")),
		mcp.WithNumber("max_bytes", mcp.Description("Cap output size in bytes (default 60000, 0 = unlimited).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := argsOf(req)
		repo, err := a.repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		rev, err := a.RequireString("revision")
		if err != nil {
			return errResult(err), nil
		}
		if err := safeRef("revision", rev); err != nil {
			return errResult(err), nil
		}
		showArgs := []string{"show", "--no-color"}
		if a.Bool("summary", false) {
			showArgs = append(showArgs, "--stat")
		}
		showArgs = append(showArgs, rev)
		out, err := runGit(ctx, repo, showArgs...)
		if err != nil {
			return errResult(err), nil
		}
		max := a.Int("max_bytes", defaultMaxBytes)
		out = truncate(out, max, "Re-run with summary=true or a larger max_bytes to see more.")
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_branch_list",
		mcp.WithDescription("List local branches with the current branch marked by '*'."),
		withRepo(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := argsOf(req).repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		r, err := openRepo(repo)
		if err != nil {
			return errResult(err), nil
		}
		head, _ := r.Head()
		var currentName string
		if head != nil {
			currentName = head.Name().Short()
		}
		iter, err := r.Branches()
		if err != nil {
			return errResult(err), nil
		}
		var names []string
		_ = iter.ForEach(func(ref *plumbing.Reference) error {
			names = append(names, ref.Name().Short())
			return nil
		})
		sort.Strings(names)
		for i, n := range names {
			if n == currentName {
				names[i] = "* " + n
			} else {
				names[i] = "  " + n
			}
		}
		return textResult(strings.Join(names, "\n")), nil
	})

	s.AddTool(mcp.NewTool("git_remote_list",
		mcp.WithDescription("List configured remotes with their URLs."),
		withRepo(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := argsOf(req).repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		r, err := openRepo(repo)
		if err != nil {
			return errResult(err), nil
		}
		remotes, err := r.Remotes()
		if err != nil {
			return errResult(err), nil
		}
		var lines []string
		for _, rem := range remotes {
			cfg := rem.Config()
			lines = append(lines, fmt.Sprintf("%s\t%s", cfg.Name, strings.Join(cfg.URLs, ",")))
		}
		return textResult(strings.Join(lines, "\n")), nil
	})

	s.AddTool(mcp.NewTool("git_tag_list",
		mcp.WithDescription("List all tags."),
		withRepo(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := argsOf(req).repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		r, err := openRepo(repo)
		if err != nil {
			return errResult(err), nil
		}
		iter, err := r.Tags()
		if err != nil {
			return errResult(err), nil
		}
		var names []string
		_ = iter.ForEach(func(ref *plumbing.Reference) error {
			names = append(names, ref.Name().Short())
			return nil
		})
		sort.Strings(names)
		return textResult(strings.Join(names, "\n")), nil
	})

	s.AddTool(mcp.NewTool("git_stash_list",
		mcp.WithDescription("List stash entries."),
		withRepo(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := argsOf(req).repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		out, err := runGit(ctx, repo, "stash", "list")
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_worktree_list",
		mcp.WithDescription("List linked worktrees."),
		withRepo(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := argsOf(req).repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		out, err := runGit(ctx, repo, "worktree", "list")
		if err != nil {
			return errResult(err), nil
		}
		return textResult(out), nil
	})

	s.AddTool(mcp.NewTool("git_head",
		mcp.WithDescription("Show the current HEAD (branch and commit)."),
		withRepo(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := argsOf(req).repoDir(defaultRepo)
		if err != nil {
			return errResult(err), nil
		}
		r, err := openRepo(repo)
		if err != nil {
			return errResult(err), nil
		}
		ref, err := r.Head()
		if err != nil {
			return errResult(err), nil
		}
		commit, err := r.CommitObject(ref.Hash())
		if err != nil {
			return errResult(err), nil
		}
		return textResult(fmt.Sprintf("%s\n%s %s\n%s", ref.Name().Short(), commit.Hash.String()[:7], commit.Author.When.Format("2006-01-02 15:04"), summary(commit))), nil
	})
}

func summary(c *object.Commit) string {
	msg := strings.TrimSpace(c.Message)
	if i := strings.IndexByte(msg, '\n'); i > 0 {
		return msg[:i]
	}
	return msg
}

// silence unused import warning when go-git types are only referenced via helpers
var _ = git.ErrRepositoryNotExists
