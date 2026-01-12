# Debugging Forkana/Gitea

This guide explains how to run the backend through **Delve (dlv)** while still using **Air** for rebuild/restart, so you can hit real breakpoints and inspect variables (e.g. request context) from GoLand or a similar IDE.

---

## Prerequisites

- Go toolchain installed (**project requires Go >= 1.25**)
- macOS Apple Silicon (arm64) supported; if you’re on Intel, skip the Rosetta section

---

## Install Delve (required)

### macOS (Apple Silicon / arm64)

Install the pinned version:

```bash
GOOS=darwin GOARCH=arm64 go install github.com/go-delve/delve/cmd/dlv@v1.26.0
```

Verify:

```bash
command -v dlv
dlv version
file "$(command -v dlv)"
```

You should see `arm64` in the `file` output.

<details>
<summary>Rosetta quick fix (macOS only)</summary>

If you see:

> `could not launch process: can not run under Rosetta`

your terminal/shell is running under Rosetta (x86_64 translation). Quick fix:

```bash
arch -arm64 zsh
arch
```

`arch` should output `arm64`.

Optional permanent fix:
- Finder → Applications → (Terminal / iTerm / GoLand) → **Get Info**
- Disable **“Open using Rosetta”**
- Quit and reopen the app
</details>

---

### Linux (Ubuntu / Debian)

Install the pinned version for the architecture you are actually running.

**Most common (x86_64 / amd64):**
```bash
GOOS=linux GOARCH=amd64 go install github.com/go-delve/delve/cmd/dlv@v1.26.0
```

**ARM64 machines (e.g. some servers, Raspberry Pi 4/5, ARM laptops):**
```bash
GOOS=linux GOARCH=arm64 go install github.com/go-delve/delve/cmd/dlv@v1.26.0
```

Verify:

```bash
command -v dlv
dlv version
file "$(command -v dlv)"
```

You should see `x86-64` (amd64) or `aarch64` (arm64) in the `file` output, matching your machine.

<details>
<summary>If <code>dlv</code> is installed but not found</summary>

Make sure your Go bin directory is on `PATH`:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.bashrc
source ~/.bashrc
command -v dlv
```

(Use `~/.zshrc` if you use zsh.)
</details>

<details>
<summary>Optional: install troubleshooting tools (lsof)</summary>

If you want to verify the debug port is open:

```bash
sudo apt-get update
sudo apt-get install -y lsof
```
</details>

---

## Start the debug watcher

Run the debug watch flow (this should start Air + Delve headless on port `2345`):

```bash
TAGS="sqlite sqlite_unlock_notify" make watch-debug
```

You should see something like:

```text
API server listening at: 127.0.0.1:2345
```

That means Delve is ready for your IDE to attach.

<details>
<summary>Optional: verify the Delve port is listening</summary>

```bash
lsof -nP -iTCP:2345 -sTCP:LISTEN
```

You should see a `dlv` process bound to `127.0.0.1:2345`.
</details>

---

## Attach from GoLand

1. **Run → Edit Configurations…**
2. Click **+** → **Go Remote**
3. Set:
   - **Host**: `127.0.0.1`
   - **Port**: `2345`
4. Ensure `make watch-debug` is running
5. Start the Go Remote configuration in **Debug** mode
6. Place breakpoints and trigger requests in the browser/API client

---

## Attach from other IDEs/editors

Any IDE/editor that supports **Delve remote attach** can connect using:

- Host: `127.0.0.1`
- Port: `2345`
- Delve API: v2 (standard)

Look for “Remote Delve”, “Go Remote”, or “Attach to Delve” in your IDE.

---

## Troubleshooting

<details>
<summary><strong>Connection refused</strong> when the IDE tries to attach</summary>

- Confirm `make watch-debug` is still running and showing Delve output.
- Verify the port is listening:

  ```bash
  lsof -nP -iTCP:2345 -sTCP:LISTEN
  ```

- If the backend exits immediately (config/port error), Delve will stop listening. Check the terminal output right after “running…” for the real cause.
</details>

<details>
<summary><strong>DWARF / debug info errors</strong> (no stack traces, can’t inspect variables)</summary>

- Ensure you installed the pinned Delve version:

  ```bash
  dlv version
  ```

- Ensure your `dlv` binary matches your system architecture (amd64 vs arm64):

  ```bash
  file "$(command -v dlv)"
  ```

- If the repo was built without debug symbols (or with stripped symbols), ensure you’re using the repo’s `watch-debug` flow, which should build a debug-friendly binary.
</details>
