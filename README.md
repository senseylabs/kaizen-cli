# Kaizen CLI

Command-line interface for [Kaizen](https://kaizen.sensey.io) board management by Sensey. Manage boards, tickets, sprints, backlogs, labels, projects, and members — all from your terminal.

**Fully interactive** — run any command without arguments and the CLI will guide you through it. Or use flags for scripting and automation.

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap senseylabs/tap
brew install kaizen-cli
```

### Go install

```bash
go install github.com/senseylabs/kaizen-cli@latest
```

### Build from source

```bash
git clone git@github.com:senseylabs/kaizen-cli.git
cd kaizen-cli
make build
# Binary at ./bin/kaizen
```

## Getting Started

### 1. Log in

```bash
kaizen login
```

You'll be prompted for your Keycloak username and password. Tokens are stored securely in the macOS Keychain (or `~/.kaizen/credentials` on Linux). Your password is never stored.

### 2. Set a default board

```bash
kaizen board set-default Sensey
```

This saves `Sensey` as your default board in `~/.kaizen/config.yaml`, so you don't need `--board` on every command.

### 3. Verify

```bash
kaizen whoami
```

### 4. Start using it

```bash
# Interactive — just run the command and follow the prompts
kaizen ticket create
kaizen ticket list
kaizen ticket update

# Or use flags for quick operations
kaizen ticket create --title "Fix login bug" --type TASK --priority HIGH --status TODO
kaizen ticket update SEN-42 --status IN_PROGRESS
```

## Interactive Mode

When you run a command **without required arguments**, the CLI enters interactive mode with prompts, numbered lists, and navigation.

### Ticket list — browse backlog or sprints

```bash
kaizen ticket list
```

```
Where to look?
  1  Backlog
  2  Sprint
Select: 2

Select a sprint:
  1  Sprint 3            ACTIVE      Jun 15 – Jun 30
  2  MCP Sprint 1        COMPLETED   Jun 03 – Jun 15
Enter number (or 'q' to quit): 1

Sprint: Sprint 3 (ACTIVE)

KEY       TITLE                   STATUS        PRIORITY    ASSIGNEE
SEN-40    Deploy pipeline          IN_PROGRESS   HIGH        Oktay Yilmaz
SEN-41    API refactor            TODO          MEDIUM      Mert Demir

Press 'b' to go back to sprint list, or 'q' to quit:
```

Combine with flags: `kaizen ticket list sprint --status IN_PROGRESS`

### Ticket create — guided prompts

```bash
kaizen ticket create
```

```
Title: Fix the login page crash
Type:
  1  TASK
  2  INCIDENT
Select: 1

Priority:
  1  LOWEST
  2  LOW
  3  MEDIUM
  4  HIGH
  5  HIGHEST
Select: 4

Status:
  1  TODO
  2  IN_PROGRESS
  3  IN_REVIEW
  4  DONE
Select: 1

Assignees? (y/n): y
  1  Emir Akyuz (emir@sensey.io)
  2  Oktay Yilmaz (oktay@sensey.io)
Select (or 'd' when done): 1
  Selected: Emir Akyuz
Select (or 'd' when done, 'r' to remove last): d

Created ticket SEN-44: Fix the login page crash
```

### Ticket update — browse, select, and modify

```bash
kaizen ticket update
```

Browses tickets interactively, lets you pick one, then choose which fields to update.

Or update directly: `kaizen ticket update SEN-42 --status DONE`

### Sprint management

```bash
kaizen sprint create         # Prompts for name, dates
kaizen sprint start          # Shows PLANNED sprints to pick from
kaizen sprint complete       # Shows ACTIVE sprints to pick from
kaizen sprint link           # Pick sprint, then multi-select tickets
```

### Comments

```bash
kaizen comment add           # Browse tickets, pick one, type comment
kaizen comment add SEN-42    # Add comment to specific ticket
```

## Flag-Based Usage (Scripting / Agents)

All commands support `--flags` and `--json` for non-interactive use:

```bash
kaizen ticket create --title "Bug fix" --type TASK --priority HIGH --status TODO --json
kaizen ticket update SEN-42 --status IN_PROGRESS --json
kaizen sprint start "Sprint 3" --json
kaizen comment add SEN-42 --content "Fixed in PR #123" --json
```

Interactive mode is **automatically disabled** when:
- `--json` flag is set
- stdin is not a terminal (pipes, CI)

## Authentication

### Interactive (humans)

```bash
kaizen login
```

### Environment variables (CI / agents)

| Variable | Description |
|----------|-------------|
| `KAIZEN_TOKEN` | Pre-obtained access token — used directly, no login needed |
| `KAIZEN_USERNAME` + `KAIZEN_PASSWORD` | Used by `kaizen login` for non-interactive authentication |

### Logout

```bash
kaizen logout
```

## Commands

### Tickets

| Command | Description |
|---------|-------------|
| `kaizen ticket list [sprint\|sprintName]` | List tickets — interactive browse or specify sprint |
| `kaizen ticket all` | List tickets across all boards |
| `kaizen ticket mine` | List tickets assigned to you |
| `kaizen ticket get [ticketKey]` | Get ticket details — browse or specify key |
| `kaizen ticket create` | Create a ticket — interactive or with flags |
| `kaizen ticket update [ticketKey]` | Update a ticket — browse or specify key |
| `kaizen ticket delete [ticketKey]` | Delete a ticket (with confirmation) |
| `kaizen ticket move [ticketKey]` | Move a ticket between boards/sprints |
| `kaizen ticket restore [ticketKey]` | Restore a deleted ticket |
| `kaizen ticket order [ticketKey]` | Reorder a ticket |
| `kaizen ticket bulk-move` | Move multiple tickets at once |

Ticket keys like `SEN-42` are resolved automatically — no UUIDs needed.

**Ticket enums:**

| Field | Values |
|-------|--------|
| `--type` | `TASK`, `INCIDENT` |
| `--priority` (TASK) | `LOWEST`, `LOW`, `MEDIUM`, `HIGH`, `HIGHEST` |
| `--priority` (INCIDENT) | `P1`, `P2`, `P3` |
| `--status` | `TODO`, `IN_PROGRESS`, `IN_REVIEW`, `DONE` |

### Sprints

| Command | Description |
|---------|-------------|
| `kaizen sprint list` | List sprints (cached 15 min) |
| `kaizen sprint get [sprintName]` | Get sprint details |
| `kaizen sprint create` | Create a sprint — interactive or with `--name` |
| `kaizen sprint update [sprintName]` | Update a sprint |
| `kaizen sprint start [sprintName]` | Start a sprint (shows PLANNED sprints) |
| `kaizen sprint complete [sprintName]` | Complete a sprint (shows ACTIVE sprints) |
| `kaizen sprint link [sprintName]` | Link tickets to a sprint |
| `kaizen sprint unlink [sprintName]` | Unlink tickets from a sprint |
| `kaizen sprint delete [sprintName]` | Delete a sprint (with confirmation) |
| `kaizen sprint restore [sprintName]` | Restore a deleted sprint |

Sprint names are resolved automatically. Dates use `YYYY-MM-DD` format.

### Comments

| Command | Description |
|---------|-------------|
| `kaizen comment list [ticketKey]` | List comments — browse tickets or specify key |
| `kaizen comment add [ticketKey]` | Add a comment |
| `kaizen comment update [ticketKey]` | Update a comment (interactive comment picker) |
| `kaizen comment delete [ticketKey]` | Delete a comment (with confirmation) |

### Boards

| Command | Description |
|---------|-------------|
| `kaizen board list` | List all boards (cached 30 min) |
| `kaizen board get <board>` | Get board details |
| `kaizen board create` | Create a new board |
| `kaizen board update <board>` | Update a board |
| `kaizen board delete <board>` | Delete a board |
| `kaizen board set-default <board>` | Set default board in config |

### Labels, Projects, Members

| Command | Description |
|---------|-------------|
| `kaizen label list` | List labels |
| `kaizen label create` | Create a label |
| `kaizen project list` | List projects |
| `kaizen project create` | Create a project |
| `kaizen member list` | List board members |
| `kaizen member add` | Add a member |

### Backlogs

| Command | Description |
|---------|-------------|
| `kaizen backlog get` | View backlog with tickets |

### Other

| Command | Description |
|---------|-------------|
| `kaizen login` | Authenticate with Keycloak |
| `kaizen logout` | Clear stored tokens and cache |
| `kaizen whoami` | Show authenticated user info |
| `kaizen cache clear` | Clear all cached data |

## Configuration

Resolved in this order (highest priority first):

1. **CLI flags** (`--api-url`, `--board`, `--org`, etc.)
2. **Environment variables** (`KAIZEN_API_URL`, `KAIZEN_ORG_ID`, `KAIZEN_DEFAULT_BOARD`)
3. **Local config** (`kaizen.yaml` in current directory)
4. **Global config** (`~/.kaizen/config.yaml`)
5. **Stored credentials** (from `kaizen login`)
6. **Defaults** (production URLs)

### Global flags

| Flag | Description |
|------|-------------|
| `--board` | Board name (uses default if not set) |
| `--json` | Output raw JSON (disables interactive mode) |
| `--debug` | Log HTTP requests (tokens redacted) |
| `--dev` | Use localhost URLs |

### Environment variables

| Variable | Description |
|----------|-------------|
| `KAIZEN_API_URL` | API base URL |
| `KAIZEN_KEYCLOAK_ISSUER` | Keycloak issuer URL |
| `KAIZEN_CLIENT_ID` | Keycloak client ID |
| `KAIZEN_CLIENT_SECRET` | Client secret |
| `KAIZEN_ORG_ID` | Organization ID |
| `KAIZEN_DEFAULT_BOARD` | Default board name |
| `KAIZEN_TOKEN` | Pre-obtained access token |
| `KAIZEN_USERNAME` / `KAIZEN_PASSWORD` | Non-interactive login |

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Error |
| `2` | Not found (404) |
| `3` | Permission denied (403) |

## Updating

```bash
brew upgrade kaizen-cli
```

The CLI will notify you when a new version is available.

## Development

```bash
make build       # Build to ./bin/kaizen
make install     # Install to $GOPATH/bin
make test        # Run tests
make lint        # Run golangci-lint
make clean       # Remove build artifacts
```

## License

MIT - see [LICENSE](LICENSE).
