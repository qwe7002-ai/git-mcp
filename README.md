# git-mcp

A Model Context Protocol (MCP) server that exposes common Git operations as tools, written in Go.

## Design

- **Read queries** (`git_status`, `git_log`, `git_diff`, `git_show`, `git_branch_list`, `git_remote_list`, `git_tag_list`, `git_head`) use `go-git` where it adds value; falling back to the `git` CLI for output that is already well-formatted (e.g. diff/log).
- **Mutations and network ops** shell out to the system `git` binary. This keeps behavior consistent with what users expect from `git`, and lets credential helpers / SSH agents / signing work without re-implementation.
- **Operating repo is chosen per call.** Every tool accepts an optional `repo` argument (absolute, or relative to the server's working directory) so the caller decides which repository each operation targets. When omitted, it falls back to the startup default set via `-repo <path>` / `GIT_MCP_REPO` / the current working directory. Caller-supplied `repo` paths are validated as real git repositories; UNC/network paths are rejected (they would trigger NTLM auth on Windows). If no default is configured and the working directory is not a repo, `repo` becomes mandatory.
- **Destructive operations** (`git_reset` with `mode=hard`, `git_push` with `force=true`, `git_clean`) require an explicit `confirm=true` parameter.

## Build

```pwsh
cd c:\Users\qwe7002\project\git-mcp
go mod tidy
go build -o git-mcp.exe .
```

## Run

```pwsh
# No default repo: every tool call must pass a "repo" argument
.\git-mcp.exe

# Set a default repo used when a call omits "repo"
.\git-mcp.exe -repo C:\path\to\repo
$env:GIT_MCP_REPO = "C:\path\to\repo"; .\git-mcp.exe
```

The server speaks MCP over stdio. The default repo is just a fallback — callers
can still target any other local repository by passing `repo` on the call.

## VS Code / Claude Desktop config example

```jsonc
{
  "mcpServers": {
    "git": {
      "command": "C:\\Users\\qwe7002\\project\\git-mcp\\git-mcp.exe",
      "args": ["-repo", "C:\\path\\to\\your\\repo"]
    }
  }
}
```

## Tools

| Category | Tool | Notes |
|---|---|---|
| Query | `git_status`, `git_log`, `git_diff`, `git_show`, `git_head` | |
| Query | `git_branch_list`, `git_remote_list`, `git_tag_list`, `git_stash_list`, `git_worktree_list` | |
| Setup | `git_clone`, `git_init` | Target directory must be local (UNC rejected); `clone` refuses non-empty targets |
| Branch | `git_branch_create`, `git_branch_delete`, `git_checkout`, `git_merge` | |
| Commit | `git_add`, `git_commit`, `git_reset` | `reset mode=hard` needs `confirm=true` |
| Remote | `git_fetch`, `git_pull`, `git_push` | `push force=true` needs `confirm=true` (uses `--force-with-lease`) |
| Remote | `git_remote_add`, `git_remote_remove`, `git_remote_set_url` | |
| Stash | `git_stash`, `git_stash_pop` | |
| Worktree | `git_worktree_add`, `git_worktree_remove` | |
| Tag | `git_tag_create` | |
| Advanced | `git_rebase`, `git_cherry_pick` | Support `abort` / `continue` flags |
| Danger | `git_clean` | Needs `confirm=true` unless `dry_run=true` |

## Notes / limitations

- Interactive rebase is not supported (no editor session over MCP).
- `git_push --force` always uses `--force-with-lease` to reduce the risk of clobbering remote work.
- The server operates on a single repository per process. Run multiple instances for multiple repos.
