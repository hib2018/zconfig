//go:build ignore

// generate_scale creates disposable large fixtures without committing a 10 MiB blob.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: go run generate_scale.go <output-directory>")
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o700); err != nil {
		panic(err)
	}
	values := make(map[string]string, 50000)
	for i := 0; i < 50000; i++ {
		values[fmt.Sprintf("item_%05d", i)] = strings.Repeat("x", 190)
	}
	data, err := json.Marshal(values)
	if err != nil {
		panic(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, "large.json"), data, 0o600); err != nil {
		panic(err)
	}
	sum := sha256.Sum256(data)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	items := make([]map[string]any, 1000)
	for i := range items {
		items[i] = map[string]any{"change_id": fmt.Sprintf("change_%04d", i), "path": fmt.Sprintf("/item_%05d", i), "operation": "replace", "expected_old": strings.Repeat("x", 190), "proposed_value": "updated", "explanation": "load fixture"}
	}
	proposal := map[string]any{"proposal_id": "large", "revision": 1, "source_digest": digest, "created_at": "2026-09-20T00:00:00Z", "items": items}
	encoded, err := json.Marshal(proposal)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "large.proposal.json"), append(encoded, '\n'), 0o600); err != nil {
		panic(err)
	}
}
