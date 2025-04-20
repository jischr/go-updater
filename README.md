# Go-Updater: Self-Updating Program

A production-quality solution to the problem of seamlessly updating deployed programs across client machines. This project demonstrates how to implement a self-updating mechanism that allows programs to automatically download and apply updates without service interruption.

In this implementation, the go-updater is running a simple server from https://github.com/jischr/simple-server. The simple-server publishes release binaries that the go-updater consumes.

## Problem Statement

Imagine that you have a program you've deployed to clients and you will periodically produce new versions. When a new version is produced, we want the deployed programs to be seamlessly replaced by a new version.

## Solution Overview

Go-Updater implements a robust solution with the following components:

1. **Self-Updating Binary**: The main program that runs on client machines and periodically checks for updates.
2. **Version Management**: Maintains multiple versions of the simple-server binary and handles the transition between versions.
3. **Reverse Proxy**: Ensures zero-downtime updates by routing traffic (via a reverse proxy) to the active binary instance.
4. **GitHub Integration**: Uses GitHub releases as the source for new versions.
5. **Cross-Platform Support**: Works on Windows, macOS, and Linux.
6. **Graceful Shutdown**: Handles termination signals properly.

## Architecture

The system consists of several components:

1. **Update Service**: Manages the update process, checking for new versions and downloading them.
2. **Version Manager**: Maintains the active binary instance and handles version transitions.
3. **GitHub Client**: Fetches release information and downloads new versions.
4. **Reverse Proxy**: Routes incoming requests to the active binary instance.

## Getting Started

### Prerequisites

- Go 1.18+

### Installation

1. Clone the repository:
   ```
   git clone https://github.com/jischr/go-updater.git
   cd go-updater
   ```

2. Build the project:
   ```
   go mod download
   go build -o go-updater
   ```

3. Run the program:
   ```
   ./go-updater
   ```

### Configuration

The program can be configured in the config/config.go file. If this was a production app, it using environment variables or a configuration file instead.

To configure the program that is executed:
- Github Owner: GitHub repository owner
- Github Repo: GitHub repository name

You can also specify
- Check Internval: Interval between update checks (30 seconds because we don't want to wait around forever in an interview)
- Proxy Port: : Port for the reverse proxy (default: 8080)

## How It Works

1. The program starts and initializes the update service and reverse proxy.
2. The update service periodically checks GitHub for new releases.
3. When a new version is available, it downloads and prepares it.
4. The version manager switches to the new version, and the reverse proxy routes traffic to it.
5. The old version is kept for a short period in case a rollback is needed.

## Flow Chart

### Simple Overview

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│             │     │             │     │             │
│  Program    │────▶│  Check for  │────▶│  New Version│
│  Running    │     │  Updates    │     │  Available? │
│             │     │             │     │             │
└─────────────┘     └─────────────┘     └─────────────┘
                                              │
                                              │ Yes
                                              ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│             │     │             │     │             │
│  Keep       │◀────│  Switch to  │◀────│  Download & │
│  Old Version│     │  New Version│     │  Start New  │
│  (Rollback) │     │             │     │  Version    │
│             │     │             │     │             │
└─────────────┘     └─────────────┘     └─────────────┘
```

### Proxy Request Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│             │     │             │     │             │
│  Client     │────▶│  Reverse    │────▶│  Get Active │
│  Request    │     │  Proxy      │     │  Version    │
│             │     │             │     │             │
└─────────────┘     └─────────────┘     └─────────────┘
                                              │
                                              ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│             │     │             │     │             │
│  No Active  │◀────│  Active     │◀────│  Forward    │
│  Version    │     │  Version    │     │  Request to │
│  Available  │     │  Found?     │     │  Active     │
│  (Error)    │     │             │     │  Version    │
│             │     │             │     │             │
└─────────────┘     └─────────────┘     └─────────────┘
        │                   │
        │                   │ Yes
        │                   ▼
        │            ┌─────────────┐
        │            │             │
        │            │  Return     │
        │            │  Response   │
        │            │             │
        │            └─────────────┘
        │
        ▼
┌─────────────┐
│             │
│  Return     │
│  503 Error  │
│             │
└─────────────┘
```

## Possible Enhancements

1. **Update Verification**: Verify checksums of Github Releases before updating
2. **Rollback Mechanism**: Implement automatic rollback if the new version fails health checks.
3. **CRON**: Use a library for managing a cron job for the updates (rather than this sleep timer)
4. **Forced Updates**: Allow admins to force an update check
5. **Metrics and Monitoring**: Add Prometheus metrics for monitoring update status.
6. **More Tests**: Add negative tests. There was only time for happy path tests.