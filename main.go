// Package main implements an MCP server exposing common git operations.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/server"

	"github.com/qwe7002/git-mcp/internal/gittools"
)

func main() {
	repoFlag := flag.String("repo", "", "default git repository path (overrides GIT_MCP_REPO); every tool also accepts a per-call \"repo\" argument")
	commitNameFlag := flag.String("commit-name", "", "force git_commit to author commits as this name (overrides GIT_MCP_COMMIT_NAME); requires -commit-email")
	commitEmailFlag := flag.String("commit-email", "", "force git_commit to author commits as this email (overrides GIT_MCP_COMMIT_EMAIL); requires -commit-name")
	flag.Parse()

	// Determine the default repository used when a tool call omits "repo".
	// An explicit -repo / GIT_MCP_REPO is validated up front (fail fast on a
	// misconfiguration). When falling back to the working directory we do not
	// require it to be a repo — the caller is expected to pass "repo" per call.
	defaultRepo := *repoFlag
	explicit := defaultRepo != ""
	if defaultRepo == "" {
		defaultRepo = os.Getenv("GIT_MCP_REPO")
		explicit = defaultRepo != ""
	}
	if defaultRepo == "" {
		if cwd, err := os.Getwd(); err == nil {
			defaultRepo = cwd
		}
	}

	if defaultRepo != "" {
		if abs, err := gittools.ResolveRepo(defaultRepo); err == nil {
			defaultRepo = abs
		} else if explicit {
			log.Fatalf("git-mcp: %v", err)
		} else {
			// cwd is not a git repo: keep it only as an absolute hint; tools
			// will require an explicit "repo" argument.
			if abs, aerr := filepath.Abs(defaultRepo); aerr == nil {
				defaultRepo = abs
			}
		}
	}

	// Forced commit identity: flag wins, then env var. Both name and email
	// must be set together — having only one would silently fall back to
	// git's default for the other, which is rarely what the operator wants.
	commitName := strings.TrimSpace(*commitNameFlag)
	if commitName == "" {
		commitName = strings.TrimSpace(os.Getenv("GIT_MCP_COMMIT_NAME"))
	}
	commitEmail := strings.TrimSpace(*commitEmailFlag)
	if commitEmail == "" {
		commitEmail = strings.TrimSpace(os.Getenv("GIT_MCP_COMMIT_EMAIL"))
	}
	if (commitName == "") != (commitEmail == "") {
		log.Fatalf("git-mcp: commit-name and commit-email must be set together (got name=%q email=%q)", commitName, commitEmail)
	}
	const forbidden = "\r\n\x00<>"
	if strings.ContainsAny(commitName, forbidden) || strings.ContainsAny(commitEmail, forbidden) {
		log.Fatalf("git-mcp: commit-name and commit-email must not contain newlines, NUL bytes, or angle brackets (< >)")
	}

	instructions := "Git MCP server. Every tool accepts an optional \"repo\" argument selecting the repository to operate on."
	if defaultRepo != "" {
		instructions += fmt.Sprintf(" When omitted, operations target: %s", defaultRepo)
	}
	if commitName != "" {
		instructions += fmt.Sprintf(" git_commit will author commits as %s <%s>.", commitName, commitEmail)
	}

	s := server.NewMCPServer(
		"git-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithInstructions(instructions),
	)

	gittools.Register(s, gittools.Options{
		DefaultRepo: defaultRepo,
		CommitName:  commitName,
		CommitEmail: commitEmail,
	})

	if err := server.ServeStdio(s); err != nil && err != context.Canceled {
		log.Fatalf("git-mcp: server error: %v", err)
	}
}
