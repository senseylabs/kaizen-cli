# Kaizen CLI

Command-line interface for [Kaizen](https://kaizen.sensey.io) board management by Sensey. Manage boards, tickets, sprints, backlogs, labels, projects, and members — all from your terminal.

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
# List your open tickets
kaizen ticket mine --status TODO,IN_PROGRESS

# Create a ticket
kaizen ticket create --title "Fix login redirect bug" --type TASK --priority HIGH --status TODO

# Update a ticket's status
kaizen ticket update Sensey <ticketId> --status IN_PROGRESS

# Add a comment
kaizen comment add Sensey <ticketId> --content "Working on this now"

# List sprints
kaizen sprint list Sensey
```

## Authentication

### Interactive (humans)

```bash
kaizen login
```

Prompts for username and password, performs a Keycloak password grant, and caches the access + refresh tokens. The CLI auto-refreshes tokens before they expire.

### Environment variables (CI / agents)

| Variable | Description |
|----------|-------------|
| `KAIZEN_TOKEN` | Pre-obtained access token — used directly, no login needed |
| `KAIZEN_USERNAME` + `KAIZEN_PASSWORD` | Auto-performs password grant at startup |

`KAIZEN_TOKEN` takes precedence over username/password. Neither credentials nor tokens are ever printed in output.

### Logout

```bash
kaizen logout
```

Clears stored tokens and local cache.

## Commands

### Auth

| Command | Description |
|---------|-------------|
| `kaizen login` | Authenticate with Keycloak |
| `kaizen logout` | Clear stored tokens and cache |
| `kaizen whoami` | Show authenticated user info |

### Boards

| Command | Description |
|---------|-------------|
| `kaizen board list [--refresh]` | List all boards (cached 30 min) |
| `kaizen board get <board>` | Get board details by name or ID |
| `kaizen board create --name <> --key <>` | Create a new board |
| `kaizen board update <board> [--name] [--key] [--description]` | Update a board |
| `kaizen board set-default <board>` | Set default board in config |
| `kaizen board related <board>` | List related boards |
| `kaizen board children add <board> --child-ids <ids>` | Add child boards |
| `kaizen board restore <board>` | Restore a deleted board |

Board names are **case-insensitive** and resolved to UUIDs automatically via cache.

### Tickets

| Command | Description |
|---------|-------------|
| `kaizen ticket list [--board] [--status] [--assignee] [--label] [--search] [--sprint] [--backlog]` | List board tickets with filters |
| `kaizen ticket all [--status] [--assignee] [--search]` | List tickets across all boards |
| `kaizen ticket mine [--status] [--search]` | List tickets assigned to you |
| `kaizen ticket get <board> <ticketId>` | Get a single ticket |
| `kaizen ticket create --title <> --type <> --priority <> --status <> [--board] [--description] [--assignee] [--label] [--project] [--sprint] [--backlog] [--story-points] [--due-date]` | Create a ticket |
| `kaizen ticket update <board> <ticketId> [--title] [--status] [--priority] [--assignee] [--label] [--story-points] [--due-date] [--percentage]` | Update ticket fields |
| `kaizen ticket move <board> <ticketId> [--target-board] [--target-sprint] [--target-backlog]` | Move a ticket |
| `kaizen ticket bulk-move <board> --tickets <ids> [--target-sprint] [--target-backlog]` | Move multiple tickets |
| `kaizen ticket order <board> <ticketId> --order <n> [--sprint] [--backlog]` | Reorder a ticket |
| `kaizen ticket restore <board> <ticketId>` | Restore a deleted ticket |

**Ticket enums:**

| Field | Values |
|-------|--------|
| `--type` | `TASK`, `INCIDENT` |
| `--priority` | `LOW`, `MEDIUM`, `HIGH`, `CRITICAL` |
| `--status` | `TODO`, `IN_PROGRESS`, `IN_REVIEW`, `DONE` |

If neither `--sprint` nor `--backlog` is specified on create, the ticket is placed in the board's default backlog automatically.

### Sprints

| Command | Description |
|---------|-------------|
| `kaizen sprint list [board] [--refresh]` | List sprints (cached 15 min) |
| `kaizen sprint get <board> <sprintId>` | Get sprint details |
| `kaizen sprint create <board> --name <> [--description] [--start-date] [--end-date]` | Create a sprint |
| `kaizen sprint update <board> <sprintId> [--name] [--description] [--start-date] [--end-date]` | Update a sprint |
| `kaizen sprint start <board> <sprintId>` | Start a sprint |
| `kaizen sprint complete <board> <sprintId>` | Complete a sprint |
| `kaizen sprint link <board> <sprintId> --tickets <ids>` | Link tickets to a sprint |
| `kaizen sprint unlink <board> <sprintId> --tickets <ids>` | Unlink tickets from a sprint |
| `kaizen sprint restore <board> <sprintId>` | Restore a deleted sprint |

Dates use `YYYY-MM-DD` format.

### Backlogs

| Command | Description |
|---------|-------------|
| `kaizen backlog get [board]` | View backlog with its tickets |
| `kaizen backlog add-ticket <board> <ticketId>` | Add a ticket to the backlog |

### Labels

| Command | Description |
|---------|-------------|
| `kaizen label list [board] [--refresh]` | List labels (cached 30 min) |
| `kaizen label create [board] --name <> [--color <>]` | Create a label |
| `kaizen label update <board> <labelId> [--name] [--color]` | Update a label |

### Projects

| Command | Description |
|---------|-------------|
| `kaizen project list [board]` | List projects |
| `kaizen project get <board> <projectId>` | Get project details |
| `kaizen project create [board] --name <> [--description] [--prefix]` | Create a project |
| `kaizen project update <board> <projectId> [--name] [--description] [--prefix]` | Update a project |

### Members

| Command | Description |
|---------|-------------|
| `kaizen member list [board] [--refresh]` | List board members (cached 15 min) |
| `kaizen member add [board] --user-id <> --role <>` | Add a member |
| `kaizen member update <board> <userId> --role <>` | Update member role |
| `kaizen member remove <board> <userId>` | Remove a member |
| `kaizen member specialties <board> <userId> --specialties <>` | Set member specialties |

### Comments

| Command | Description |
|---------|-------------|
| `kaizen comment list <board> <ticketId>` | List comments on a ticket |
| `kaizen comment add <board> <ticketId> --content <>` | Add a comment |
| `kaizen comment update <board> <ticketId> <commentId> --content <>` | Update a comment |

### Cache

| Command | Description |
|---------|-------------|
| `kaizen cache clear` | Clear all cached data |

Use `--refresh` on any list command to bypass cache for that request.

## Configuration

Resolved in this order (highest priority first):

1. **CLI flags** (`--api-url`, `--board`, `--org`, etc.)
2. **Environment variables** (`KAIZEN_API_URL`, `KAIZEN_ORG_ID`, `KAIZEN_DEFAULT_BOARD`)
3. **Local config** (`kaizen.yaml` in current directory)
4. **Global config** (`~/.kaizen/config.yaml`)
5. **Stored credentials** (from `kaizen login`)
6. **Defaults** (production URLs)

### Config file example

```yaml
# ~/.kaizen/config.yaml
api-url: https://api.village.sensey.io
issuer: https://keycloak.sensey.io/realms/sensey
default-board: Sensey
```

### Global flags

| Flag | Env var | Default |
|------|---------|---------|
| `--api-url` | `KAIZEN_API_URL` | `https://api.village.sensey.io` |
| `--issuer` | `KAIZEN_KEYCLOAK_ISSUER` | `https://keycloak.sensey.io/realms/sensey` |
| `--org` | `KAIZEN_ORG_ID` | (from login) |
| `--board` | `KAIZEN_DEFAULT_BOARD` | (from config) |
| `--json` | — | Output raw JSON |
| `--dev` | — | Use localhost URLs |
| `--debug` | — | Log HTTP requests (tokens redacted) |

### Dev mode

Pass `--dev` to target your local environment:

```bash
kaizen --dev login
kaizen --dev ticket list
```

Dev mode defaults: API `http://localhost:8080`, Keycloak `http://localhost:8086/realms/sensey`.

## JSON Output

Pass `--json` to any command for machine-readable output:

```bash
# Get ticket as JSON
kaizen ticket get Sensey <ticketId> --json

# Pipe to jq
kaizen ticket mine --status TODO --json | jq '.[].key'

# Use in scripts
TICKET_ID=$(kaizen ticket create --title "Test" --type TASK --priority LOW --status TODO --json | jq -r '.id')
```

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Error (auth, API, validation) |
| `2` | Not found (404) |
| `3` | Permission denied (403) |

## Caching

The CLI caches metadata to `~/.kaizen/cache.json` to minimize API calls:

| Resource | TTL | Cache key |
|----------|-----|-----------|
| Boards | 30 min | `boards` |
| Backlogs | 60 min | `backlog:{boardId}` |
| Members | 15 min | `members:{boardId}` |
| Labels | 30 min | `labels:{boardId}` |
| Sprints | 15 min | `sprints:{boardId}` |

Tickets are **never cached** — always fetched live.

Board names (e.g., "Sensey") are resolved to UUIDs via the board cache, so you rarely need to type a UUID.

## Examples

```bash
# List all boards
kaizen board list

# Create a high-priority ticket assigned to someone
kaizen ticket create --title "Critical: API timeout" --type INCIDENT --priority CRITICAL --status TODO --assignee <userId>

# Search for tickets
kaizen ticket list --search "invoice" --status TODO,IN_PROGRESS

# Create a sprint and link tickets
kaizen sprint create Sensey --name "Sprint 14" --start-date 2026-04-20 --end-date 2026-05-03
kaizen sprint link Sensey <sprintId> --tickets <id1>,<id2>,<id3>
kaizen sprint start Sensey <sprintId>

# Check who's on a board
kaizen member list Sensey

# View and comment on a ticket
kaizen ticket get Sensey <ticketId>
kaizen comment add Sensey <ticketId> --content "Reviewed — looks good, merging now"
```

## Development

```bash
make build       # Build to ./bin/kaizen
make install     # Install to $GOPATH/bin
make test        # Run tests
make lint        # Run golangci-lint
make dev         # Build with -dev version suffix
make clean       # Remove build artifacts
```

## License

MIT - see [LICENSE](LICENSE).
