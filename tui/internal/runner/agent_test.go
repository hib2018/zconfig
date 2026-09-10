package runner

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/hib2018/zconfig/tui/internal/protocol"
)

func TestAgentHelper(t *testing.T) {
	if os.Getenv("GO_WANT_AGENT_HELPER") != "1" {
		return
	}
	var request protocol.Request
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		os.Exit(2)
	}
	if request.Operation == protocol.OperationProtocolInfo {
		json.NewEncoder(os.Stdout).Encode(protocol.Response{ProtocolVersion: protocol.Version, RequestID: request.RequestID, OK: true, ResultSchema: "zconfig.protocol-info/1", Result: json.RawMessage(`{"protocol_versions":["1.0"],"operations":["revise_proposal"],"schemas":["zconfig.revision/1"],"limits":{"message_bytes":16777216}}`)})
		os.Exit(0)
	}
	json.NewEncoder(os.Stdout).Encode(protocol.Response{ProtocolVersion: protocol.Version, RequestID: request.RequestID, OK: true, ResultSchema: "zconfig.revision/1", Result: json.RawMessage(`{"proposal_id":"p","base_revision":1,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","proposal":{"proposal_id":"p","revision":2,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:00:00Z","items":[{"change_id":"a","path":"/a","operation":"add","proposed_value":2,"explanation":"revised"}]},"resolved_comment_ids":["c1"]}`)})
	os.Exit(0)
}

func TestAgentHandshakeAndOneShotRevision(t *testing.T) {
	command := Command{Executable: os.Args[0], Args: []string{"-test.run=TestAgentHelper"}, Environment: map[string]string{"GO_WANT_AGENT_HELPER": "1"}, Timeout: 5 * time.Second}
	agent := Agent{Command: command}
	if err := agent.Handshake(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "r1", Operation: protocol.OperationReviseProposal, PayloadSchema: "zconfig.revision/1", Payload: json.RawMessage(`{}`)}
	result, err := agent.Revise(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Proposal.Revision != 2 || len(result.ResolvedCommentIDs) != 1 {
		t.Fatalf("result=%+v", result)
	}
}
