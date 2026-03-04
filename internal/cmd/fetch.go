package cmd

import (
	"time"

	"github.com/maxcelant/git-synced/internal/config"
	"github.com/maxcelant/git-synced/internal/fetch"
	"github.com/maxcelant/git-synced/internal/providers"
)

// FetchEntries delegates to the fetch package so other packages can use it
// without creating an import cycle through internal/cmd.
func FetchEntries(cfg config.Config, from, until time.Time) ([]providers.Entry, []string, int, error) {
	return fetch.Entries(cfg, from, until)
}
