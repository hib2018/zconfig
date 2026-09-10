package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hib2018/zconfig/tui/internal/protocol"
)

type Agent struct{ Command Command }

func (a Agent) Handshake(ctx context.Context) error {
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "agent-handshake", Operation: protocol.OperationProtocolInfo, PayloadSchema: "zconfig.empty/1", Payload: json.RawMessage(`{}`)}
	response, err := RunProtocol(ctx, a.Command, request, nil)
	if err != nil {
		return err
	}
	if !response.OK {
		return fmt.Errorf("agent handshake rejected: %s", response.Error.Code)
	}
	var info protocol.ProtocolInfo
	if err := protocol.DecodePayload(response.Result, &info); err != nil {
		return err
	}
	for _, operation := range info.Operations {
		if operation == string(protocol.OperationReviseProposal) {
			return nil
		}
	}
	return errors.New("agent does not advertise revise_proposal")
}

func (a Agent) Revise(ctx context.Context, request protocol.Request, disclosedSecrets []string) (protocol.RevisionResultPayload, error) {
	if request.Operation != protocol.OperationReviseProposal {
		return protocol.RevisionResultPayload{}, errors.New("not a revision request")
	}
	response, err := RunProtocol(ctx, a.Command, request, disclosedSecrets)
	if err != nil {
		return protocol.RevisionResultPayload{}, err
	}
	if !response.OK {
		return protocol.RevisionResultPayload{}, fmt.Errorf("agent revision rejected: %s", response.Error.Code)
	}
	if response.ResultSchema != "zconfig.revision/1" {
		return protocol.RevisionResultPayload{}, errors.New("unexpected revision result schema")
	}
	var result protocol.RevisionResultPayload
	if err := protocol.DecodePayload(response.Result, &result); err != nil {
		return protocol.RevisionResultPayload{}, err
	}
	return result, nil
}
