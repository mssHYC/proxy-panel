package firewall

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

type ufwBackend struct {
	run runner
}

func newUFWBackend(r runner) Backend { return &ufwBackend{run: r} }

func (u *ufwBackend) Name() string { return "ufw" }

func (u *ufwBackend) Available(ctx context.Context) error {
	stdout, _, err := u.run(ctx, "ufw", "status")
	if err != nil {
		return err
	}
	if !bytes.Contains(stdout, []byte("Status: active")) {
		return &backendUnavailable{backend: "ufw", reason: "status 不是 active"}
	}
	return nil
}

func (u *ufwBackend) Allow(ctx context.Context, port int) error {
	for _, proto := range []string{"tcp", "udp"} {
		_, _, err := u.run(ctx, "ufw", "allow", strconv.Itoa(port)+"/"+proto)
		if err != nil {
			return err
		}
	}
	return nil
}

func (u *ufwBackend) Revoke(ctx context.Context, port int) error {
	for _, proto := range []string{"tcp", "udp"} {
		_, stderr, err := u.run(ctx, "ufw", "delete", "allow", strconv.Itoa(port)+"/"+proto)
		if err != nil {
			if isUFWNonExistent(stderr) {
				continue
			}
			return err
		}
	}
	return nil
}

func isUFWNonExistent(stderr []byte) bool {
	return strings.Contains(string(stderr), "Could not delete non-existent rule")
}

type backendUnavailable struct {
	backend string
	reason  string
}

func (e *backendUnavailable) Error() string { return e.backend + " 不可用: " + e.reason }
