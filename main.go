// Package main implements an MCP server exposing common git operations.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"

	"github.com/qwe7002/git-mcp/internal/gittools"
)

func main() {
	repoFlag := flag.String("repo", "", "default git repository path (overrides GIT_MCP_REPO); every tool also accepts a per-call \"repo\" argument")
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

	instructions := "Git MCP server. Every tool accepts an optional \"repo\" argument selecting the repository to operate on."
	if defaultRepo != "" {
		instructions += fmt.Sprintf(" When omitted, operations target: %s", defaultRepo)
	}

	s := server.NewMCPServer(
		"git-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithInstructions(instructions),
	)

	gittools.Register(s, defaultRepo)

	if err := server.ServeStdio(s); err != nil && err != context.Canceled {
		log.Fatalf("git-mcp: server error: %v", err)
	}
}
