// Package main implements an MCP server exposing common git operations.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/qwe7002/git-mcp/internal/gittools"
)

func main() {
	repoFlag := flag.String("repo", "", "path to the git repository (overrides GIT_MCP_REPO)")
	flag.Parse()

	repoPath := *repoFlag
	if repoPath == "" {
		repoPath = os.Getenv("GIT_MCP_REPO")
	}
	if repoPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("git-mcp: cannot determine working directory: %v", err)
		}
		repoPath = cwd
	}

	abs, err := gittools.ResolveRepo(repoPath)
	if err != nil {
		log.Fatalf("git-mcp: %v", err)
	}

	s := server.NewMCPServer(
		"git-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithInstructions(fmt.Sprintf("Git MCP server operating on repository: %s", abs)),
	)

	gittools.Register(s, abs)

	if err := server.ServeStdio(s); err != nil && err != context.Canceled {
		log.Fatalf("git-mcp: server error: %v", err)
	}
}
