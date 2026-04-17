# Kaizen CLI — Quick Setup Guide

## 1. Install

```bash
brew tap senseylabs/tap
brew install kaizen-cli
```

Verify installation:

```bash
kaizen --version
```

## 2. Get Your Credentials

Your login credentials are stored in **Kagi** under:

**Password** > **Kaizen-cli**

You'll need the **username** and **password** from that entry.

## 3. Login

```bash
kaizen login
```

Enter your username and password when prompted. You only need to do this once — credentials are stored securely in your macOS Keychain.

## 4. Set Default Board

```bash
kaizen board set-default Sensey
```

This saves `Sensey` as your default board so you don't need to specify it on every command.

## 5. Verify

```bash
kaizen board list
```

You should see the list of boards you have access to.

## 6. Common Commands

The CLI is **fully interactive** — just run a command without arguments and it will guide you.

```bash
# Create a ticket (interactive prompts for all fields)
kaizen ticket create

# Browse and list tickets (backlog or sprint picker)
kaizen ticket list

# Update a ticket (browse, pick, choose what to update)
kaizen ticket update

# Or use ticket keys directly
kaizen ticket get SEN-42
kaizen ticket update SEN-42 --status IN_PROGRESS

# Sprint management
kaizen sprint create
kaizen sprint start
kaizen sprint complete

# Add a comment
kaizen comment add SEN-42

# View your tickets
kaizen ticket mine
```

## 7. Updating

```bash
brew upgrade kaizen-cli
```

The CLI will notify you when a new version is available.

## Need Help?

```bash
kaizen --help
kaizen <command> --help
```
