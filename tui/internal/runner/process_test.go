package runner

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hib2018/zconfig/tui/internal/protocol"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	mode := os.Getenv("HELPER_MODE")
	switch mode {
	case "success":
		os.Stdout.WriteString(`{"protocol_version":"1.0","request_id":"r1","ok":true,"result_schema":"zconfig.protocol-info/1","result":{}}`)
	case "extra":
		os.Stdout.WriteString(`{"protocol_version":"1.0","request_id":"r1","ok":true,"result_schema":"zconfig.protocol-info/1","result":{}} trailing`)
	case "wrong-id":
		os.Stdout.WriteString(`{"protocol_version":"1.0","request_id":"wrong","ok":true,"result_schema":"zconfig.protocol-info/1","result":{}}`)
	case "large":
		os.Stdout.WriteString(strings.Repeat("x", 256))
	case "secret":
		os.Stderr.WriteString("failed hunter2")
		os.Exit(3)
	case "sleep":
		time.Sleep(5 * time.Second)
	case "nonzero":
		os.Stderr.WriteString("boom")
		os.Exit(2)
	}
	os.Exit(0)
}

func helper(mode string) Command {
	return Command{Executable: os.Args[0], Args: []string{"-test.run=TestHelperProcess"}, Environment: map[string]string{"GO_WANT_HELPER_PROCESS": "1", "HELPER_MODE": mode}, Timeout: 5 * time.Second, StdoutLimit: 128, StderrLimit: 128}
}

func TestRunProtocolFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper exit semantics are covered in CI integration")
	}
	req := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "r1", Operation: protocol.OperationProtocolInfo, PayloadSchema: "zconfig.empty/1", Payload: []byte(`{}`)}
	for _, tc := range []struct {
		name string
		want error
	}{
		{"extra", ErrInvalidOutput}, {"wrong-id", ErrInvalidOutput}, {"large", ErrOutputLimit}, {"nonzero", ErrNonZeroExit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RunProtocol(context.Background(), helper(tc.name), req, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRunProtocolUsesDefaultOutputLimit(t *testing.T) {
	command := helper("success")
	command.StdoutLimit = 0
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "r1", Operation: protocol.OperationProtocolInfo, PayloadSchema: "zconfig.empty/1", Payload: []byte(`{}`)}
	if _, err := RunProtocol(context.Background(), command, request, nil); err != nil {
		t.Fatal(err)
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	cmd := helper("sleep")
	cmd.Timeout = 20 * time.Millisecond
	_, err := Run(context.Background(), cmd, nil, nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Run(ctx, helper("sleep"), nil, nil)
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("got %v", err)
	}
}

func TestDiagnosticsAreSanitized(t *testing.T) {
	_, err := Run(context.Background(), helper("secret"), nil, []string{"hunter2"})
	if err == nil || strings.Contains(err.Error(), "hunter2") || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("unsafe error: %v", err)
	}
}
