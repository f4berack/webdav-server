package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// meta is stored as <calendar>/.meta.json
type meta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (b *backend) writeMeta(slug string, m meta) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(b.calDir(slug), ".meta.json"), data, 0o644)
}
