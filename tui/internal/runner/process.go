package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/hib2018/zconfig/tui/internal/protocol"
)

var (
	ErrTimeout       = errors.New("process deadline exceeded")
	ErrCanceled      = errors.New("process canceled")
	ErrOutputLimit   = errors.New("process output exceeds limit")
	ErrNonZeroExit   = errors.New("process exited unsuccessfully")
	ErrInvalidOutput = errors.New("invalid protocol output")
)

type Command struct {
	Executable       string
	Args             []string
	WorkingDirectory string
	Environment      map[string]string
	EnvironmentList  []string
	Timeout          time.Duration
	StdoutLimit      int64
	StderrLimit      int64
}
type Result struct {
	Stdout, Stderr []byte
	ExitCode       int
}
type boundedBuffer struct {
	bytes.Buffer
	limit    int64
	exceeded bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if int64(b.Len()+len(p)) > b.limit {
		remaining := int(b.limit) - b.Len()
		if remaining > 0 {
			_, _ = b.Buffer.Write(p[:remaining])
		}
		b.exceeded = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

func Run(parent context.Context, command Command, stdin []byte, secrets []string) (Result, error) {
	if command.Executable == "" {
		return Result{}, errors.New("missing executable")
	}
	if command.Timeout <= 0 {
		command.Timeout = 10 * time.Second
	}
	if command.StdoutLimit <= 0 {
		command.StdoutLimit = protocol.MaxMessageBytes
	}
	if command.StderrLimit <= 0 {
		command.StderrLimit = protocol.MaxMessageBytes
	}
	ctx, cancel := context.WithTimeout(parent, command.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command.Executable, command.Args...)
	cmd.Dir = command.WorkingDirectory
	if command.EnvironmentList != nil {
		cmd.Env = append([]string(nil), command.EnvironmentList...)
	} else if command.Environment != nil {
		cmd.Env = make([]string, 0, len(command.Environment))
		for k, v := range command.Environment {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	cmd.Stdin = bytes.NewReader(stdin)
	out := &boundedBuffer{limit: command.StdoutLimit}
	diagnostic := &boundedBuffer{limit: command.StderrLimit}
	cmd.Stdout = out
	cmd.Stderr = diagnostic
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	result := Result{Stdout: protocol.Sanitize(out.Bytes(), secrets), Stderr: protocol.Sanitize(diagnostic.Bytes(), secrets)}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if out.exceeded || diagnostic.exceeded {
		return result, fmt.Errorf("%w: stdout=%t stderr=%t", ErrOutputLimit, out.exceeded, diagnostic.exceeded)
	}
	if errors.Is(parent.Err(), context.Canceled) {
		return result, ErrCanceled
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return result, ErrTimeout
	}
	if err != nil {
		return result, fmt.Errorf("%w (exit %d): %s", ErrNonZeroExit, result.ExitCode, result.Stderr)
	}
	return result, nil
}

func RunProtocol(ctx context.Context, command Command, request protocol.Request, secrets []string) (protocol.Response, error) {
	decodeLimit := command.StdoutLimit
	if decodeLimit <= 0 {
		decodeLimit = protocol.MaxMessageBytes
	}
	input, err := json.Marshal(request)
	if err != nil {
		return protocol.Response{}, err
	}
	result, err := Run(ctx, command, input, secrets)
	if err != nil {
		return protocol.Response{}, err
	}
	response, err := protocol.DecodeResponse(bytes.NewReader(result.Stdout), decodeLimit, request.RequestID)
	if err != nil {
		if command.StdoutLimit > 0 && int64(len(result.Stdout)) >= command.StdoutLimit {
			return protocol.Response{}, ErrOutputLimit
		}
		return protocol.Response{}, fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}
	return response, nil
}

var _ io.Writer = (*boundedBuffer)(nil)
