package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

type request struct {
	ProtocolVersion string          `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	Operation       string          `json:"operation"`
	Payload         json.RawMessage `json:"payload"`
}

func main() {
	mode := flag.String("mode", "success", "fixture behavior")
	flag.Parse()
	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		os.Exit(2)
	}
	if req.Operation == "protocol_info" {
		write(req.RequestID, "zconfig.protocol-info/1", map[string]any{"protocol_versions": []string{"1.0"}, "operations": []string{"revise_proposal"}, "schemas": []string{"zconfig.revision/1"}, "limits": map[string]int{"message_bytes": 16 << 20}})
		return
	}
	switch *mode {
	case "timeout":
		time.Sleep(10 * time.Second)
	case "malformed":
		fmt.Fprint(os.Stdout, "not json")
	case "secret-echo":
		fmt.Fprint(os.Stderr, "fixture-secret")
		os.Exit(3)
	case "nonzero":
		os.Exit(4)
	case "success", "scope-expansion":
		var payload map[string]any
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			os.Exit(2)
		}
		proposal := payload["proposal"].(map[string]any)
		proposal["revision"] = proposal["revision"].(float64) + 1
		items := proposal["items"].([]any)
		allowed := map[string]bool{}
		for _, value := range payload["allowed_change_item_ids"].([]any) {
			allowed[value.(string)] = true
		}
		for _, raw := range items {
			item := raw.(map[string]any)
			if (*mode == "success" && allowed[item["change_id"].(string)]) || (*mode == "scope-expansion" && !allowed[item["change_id"].(string)]) {
				item["proposed_value"] = "fixture-revised"
			}
		}
		comments := payload["comments"].([]any)
		resolved := make([]string, 0, len(comments))
		for _, raw := range comments {
			resolved = append(resolved, raw.(map[string]any)["comment_id"].(string))
		}
		write(req.RequestID, "zconfig.revision/1", map[string]any{"proposal_id": payload["proposal_id"], "base_revision": payload["base_revision"], "source_digest": payload["source_digest"], "proposal": proposal, "resolved_comment_ids": resolved})
	default:
		os.Exit(2)
	}
}

func write(requestID, schema string, result any) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol_version": "1.0", "request_id": requestID, "ok": true, "result_schema": schema, "result": result})
}
