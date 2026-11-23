# jfsh (IINA fork)

A terminal-based client for [Jellyfin](https://jellyfin.org) that lets you browse your
media library and play videos in [IINA](https://iina.io). Intended to be used alongside the
Jellyfin IINA Plugin [mhajder/iina-jellyfin](https://github.com/mhajder/iina-jellyfin)

> **Note:** Unlike upstream, this fork does not sync playback progress with Jellyfin. That is
> instead delegated to the [iina-jellyfin](https://github.com/mhajder/iina-jellyfin) plugin.

Private fork of [hacel/jfsh](https://github.com/hacel/jfsh), which plays through
[mpv](https://mpv.io) instead. Upstream was inspired by
[jftui](https://github.com/Aanok/jftui).

![Demo](demo/demo.gif)

> The demo above is upstream's and still shows mpv.

## How this differs from upstream

Upstream drives mpv as a long-lived process over mpv's JSON IPC socket, which is how it
reports progress, resumes, queues the next episode, loads subtitles and skips intros.
IINA embeds mpv through the client API rather than the terminal, so it cannot stand in
for that socket.

It does not need to. This fork hands the stream URL to IINA and exits, leaving playback
state to the [iina-jellyfin](https://github.com/mhajder/iina-jellyfin) plugin, which
recognizes the item from the `api_key` in the URL. The mpv IPC client is deleted rather
than ported.

Concretely:

- Playback is a one-shot `iina --no-stdin <url>`. The browser returns immediately
  instead of blocking until the player exits.
- Stream URLs are `/Videos/{id}/stream?static=true&api_key={token}`. The capitalized
  route and the token are what the plugin matches on; upstream's lowercase tokenless URL
  is ignored by it. The token is not needed to stream, only to be recognized.
- `skip_segments` is no longer read. Intro skipping comes from the IINA side.
- **macOS only**, because IINA is. Upstream supports Linux and Windows.

## Features

- Browse your Jellyfin library and play in IINA
- No mouse required

These come from the IINA plugins rather than from jfsh:

- Resumes playback, and reports progress back to Jellyfin
- Autoplays the next episode
- Picks preferred audio and subtitle tracks
  
## Installation

### Prerequisites

- A running [Jellyfin](https://jellyfin.org) instance.
- [IINA](https://iina.io), with `iina` in `PATH`. The Homebrew cask symlinks it:

  ```sh
  brew install --cask iina
  ```

- The [iina-jellyfin](https://github.com/mhajder/iina-jellyfin) plugin, installed in
  IINA and logged in to the same server. Without it, video still plays but nothing is
  reported back to Jellyfin.

mpv is not required.

### Install

From a clone of this repository:

```sh
go install .
```

Upstream's AUR package, `nix run github:hacel/jfsh` and the GitHub releases all install
*upstream* jfsh, which uses mpv, so they are not install paths for this fork.

## Usage

1. **Start jfsh**

   ```sh
   jfsh
   ```

2. **Login**

   On first launch, you'll be prompted to enter:

   - **Host**: e.g., `http://localhost:8096`
   - **Username**
   - **Password**

3. **Play Media**

   - Select an item and press **Enter** or **Space** to play it.
   - IINA opens in its own window and jfsh returns to the browser.

4. **Quit**

   - Press **`q`** at any time to exit jfsh.

## Configuration

By default, the configuration file is stored in `$XDG_CONFIG_HOME/jfsh/jfsh.yaml`. If
`$XDG_CONFIG_HOME` is not set it defaults to `~/Library/Application Support/jfsh/jfsh.yaml`
on macOS.

```yaml
host: http://localhost:8096
username: me
device: mycomputer # Device name to report to jellyfin (default: hostname)
password_file: /run/secrets/jfsh-password # Optional runtime secret file
```

When no reusable token or `password_file` is available, jfsh prompts for the
password. Passwords are never stored by jfsh. Authentication tokens and the
generated device ID are stored in `$XDG_STATE_HOME/jfsh/state.yaml`.

A leftover `skip_segments` key in an existing config file is ignored rather than
rejected, so upstream configs still load.

## Plans

- Configuration through TUI
- Complete library browsing
- Sorting
- Better search: Filter by media type, watched status, and metadata
