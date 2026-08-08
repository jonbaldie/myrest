package verification

import (
	"fmt"
	"os"
	"path/filepath"
)

// ChapterFiles are the capability-area markdown files that own Gap list rows.
// Verification derives the gap list from these files only.
var ChapterFiles = []string{
	"auth.md",
	"schema-cache.md",
	"read-parity-boundaries.md",
	"aggregates.md",
	"embed.md",
	"write.md",
	"rpc-get.md",
	"rpc-body-modes.md",
	"rpc-row-set.md",
	"media-types-and-prefer.md",
	"error-contract.md",
	"discovery.md",
	"transactions.md",
	"cors-and-proxy.md",
	"config.md",
}

// LoadChapters reads capability chapter markdown from a docs directory.
func LoadChapters(docsDir string) (map[string]string, error) {
	chapters := make(map[string]string, len(ChapterFiles))
	for _, name := range ChapterFiles {
		path := filepath.Join(docsDir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		chapters[name] = string(body)
	}
	return chapters, nil
}
