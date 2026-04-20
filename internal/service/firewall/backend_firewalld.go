package firewall

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

type firewalldBackend struct {
	run runner
}

func newFirewalldBackend(r runner) Backend { return &firewalldBackend{run: r} }

func (f *firewalldBackend) Name() string { return "firewalld" }

func (f *firewalldBackend) Available(ctx context.Context) error {
	stdout, _, err := f.run(ctx, "firewall-cmd", "--state")
	if err != nil {
		return err
	}
	if !bytes.Contains(stdout, []byte("running")) {
		return &backendUnavailable{backend: "firewalld", reason: "状态不是 running"}
	}
	return nil
}

func (f *firewalldBackend) Allow(ctx context.Context, port int) error {
	for _, proto := range []string{"tcp", "udp"} {
		stdout, _, err := f.run(ctx, "firewall-cmd", "--permanent",
			"--add-port="+strconv.Itoa(port)+"/"+proto)
		if err != nil && !isFirewalldAlreadyEnabled(stdout) {
			return err
		}
	}
	return f.reload(ctx)
}

func (f *firewalldBackend) Revoke(ctx context.Context, port int) error {
	for _, proto := range []string{"tcp", "udp"} {
		stdout, _, err := f.run(ctx, "firewall-cmd", "--permanent",
			"--remove-port="+strconv.Itoa(port)+"/"+proto)
		if err != nil && !isFirewalldNotEnabled(stdout) {
			return err
		}
	}
	return f.reload(ctx)
}

func (f *firewalldBackend) reload(ctx context.Context) error {
	_, _, err := f.run(ctx, "firewall-cmd", "--reload")
	return err
}

func isFirewalldAlreadyEnabled(stdout []byte) bool {
	return strings.Contains(string(stdout), "ALREADY_ENABLED")
}

func isFirewalldNotEnabled(stdout []byte) bool {
	return strings.Contains(string(stdout), "NOT_ENABLED")
}
