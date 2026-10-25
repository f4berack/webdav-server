package main

import (
	"context"
	"os"
)

// It creates the data folder and the "personal" calendar on first start.
func (b *backend) ensureDefault() error {

	// 1. make sure ./data exists (like mkdir -p)
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return err
	}

	// 2. if at least one calendar already exists, there is nothing to do
	cals, err := b.ListCalendars(context.Background())
	if err != nil {
		return err
	}
	if len(cals) > 0 {
		return nil
	}

	// 3. first start: create "personal" with its metadata
	if err := os.Mkdir(b.calDir("personal"), 0o755); err != nil {
		return err
	}

	return b.writeMeta("personal", meta{Name: "Personal"})

}
