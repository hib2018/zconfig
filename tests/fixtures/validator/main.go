package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
)

type request struct {
	CandidatePath   string `json:"candidate_path"`
	CandidateDigest string `json:"candidate_digest"`
	SourceDigest    string `json:"source_digest"`
}

func main() {
	mode := flag.String("mode", "passed", "validator result")
	flag.Parse()
	var input request
	if json.NewDecoder(os.Stdin).Decode(&input) != nil {
		os.Exit(2)
	}
	contents, err := os.ReadFile(input.CandidatePath)
	if err != nil {
		os.Exit(3)
	}
	sum := sha256.Sum256(contents)
	if "sha256:"+hex.EncodeToString(sum[:]) != input.CandidateDigest {
		os.Exit(4)
	}
	status := *mode
	if status != "passed" && status != "failed" && status != "unverified" {
		os.Exit(5)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"candidate_digest": input.CandidateDigest, "status": status, "code": "fixture." + status, "message": "fixture validator result"})
}
