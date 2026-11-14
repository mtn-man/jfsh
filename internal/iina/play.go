// Package iina opens jellyfin items in IINA.
//
// Playback state is deliberately not tracked here. Progress reporting, resume,
// autoplay of the next episode, track selection and intro skipping are all
// handled by the iina-jellyfin plugin, which picks the item up from the
// api_key in the stream URL, so this only has to launch the player.
package iina

import (
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/hacel/jfsh/internal/config"
	"github.com/hacel/jfsh/internal/jellyfin"
)

// player is IINA's command line tool, symlinked as "iina" by the Homebrew cask.
const player = "iina"

func Play(session *config.Session, items []jellyfin.Item, index int) error {
	if _, err := exec.LookPath(player); err != nil {
		return fmt.Errorf("%s not found in PATH, install it with: brew install --cask iina", player)
	}

	item := items[index]
	url, err := jellyfin.GetStreamingURL(session.Host, session.Token, item)
	if err != nil {
		return fmt.Errorf("failed to build streaming url: %w", err)
	}

	// --no-stdin keeps iina-cli off the terminal bubbletea is drawing to.
	// Without --keep-running it launches IINA and exits immediately, so the
	// browser comes back while playback continues in IINA's own window.
	if err := exec.Command(player, "--no-stdin", url).Run(); err != nil {
		return fmt.Errorf("failed to launch %s: %w", player, err)
	}

	slog.Info("opened item in IINA", "item", item.GetName())
	return nil
}
