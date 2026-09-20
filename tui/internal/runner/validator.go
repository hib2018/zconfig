package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hib2018/zconfig/tui/internal/protocol"
)

type Validator struct {
	Name    string
	Command Command
}

func (v Validator) Validate(ctx context.Context, request protocol.ExternalValidatorRequest, disclosedSecrets []string) (protocol.ExternalValidatorResult, error) {
	if v.Name == "" {
		return protocol.ExternalValidatorResult{}, errors.New("validator name is required")
	}
	input, err := json.Marshal(request)
	if err != nil {
		return protocol.ExternalValidatorResult{}, err
	}
	if err := protocol.ValidateContract(protocol.ContractValidatorRequest, input, ""); err != nil {
		return protocol.ExternalValidatorResult{}, fmt.Errorf("validator request: %w", err)
	}
	result, err := Run(ctx, v.Command, input, disclosedSecrets)
	if err != nil {
		return protocol.ExternalValidatorResult{}, fmt.Errorf("validator %s: %w", v.Name, err)
	}
	if err := protocol.ValidateContract(protocol.ContractValidatorResult, result.Stdout, request.CandidateDigest); err != nil {
		return protocol.ExternalValidatorResult{}, fmt.Errorf("validator %s output: %w", v.Name, err)
	}
	var response protocol.ExternalValidatorResult
	if err := protocol.DecodePayload(result.Stdout, &response); err != nil {
		return protocol.ExternalValidatorResult{}, fmt.Errorf("validator %s output: %w", v.Name, err)
	}
	if response.Status != "passed" {
		return response, fmt.Errorf("validator %s blocked final assembly: %s", v.Name, response.Code)
	}
	return response, nil
}

func UnverifiedValidatorResult(candidateDigest, code, message string) protocol.ExternalValidatorResult {
	return protocol.ExternalValidatorResult{CandidateDigest: candidateDigest, Status: "unverified", Code: code, Message: message}
}
