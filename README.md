# Backuper Agent

A lightweight Go-based backup agent that automatically copies local files to an SMB network share and sends status notifications via Telegram.

## How It Works

The agent reads a JSON configuration file that defines backup jobs (sets of local paths to back up). It connects to an SMB share, creates timestamped backup directories, copies all configured files and folders, and automatically rotates old versions to save space. After each backup run, it sends a detailed report to Telegram with statistics like file counts, sizes, and available disk space.

## Building

```bash
# Build for your platform
make build

# Build for all platforms (Windows, Linux, macOS)
make build-all
```

The compiled binary will be in the `./build` directory.

## Running

1. Copy `config.json.example` to `config.json` and configure your SMB share, Telegram credentials, and backup paths
2. Run the agent:

```bash
./backuper-agent
```

Or specify a custom config path:

```bash
./backuper-agent -configPath=/path/to/config.json
```

See `config.json.example` for configuration details.