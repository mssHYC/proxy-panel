package routing_test

import (
	"context"
	"errors"
	"testing"

	"proxy-panel/internal/service/routing"
)

func TestCustomRuleInput_Validate(t *testing.T) {
	id := int64(1)
	cases := []struct {
		name string
		in   routing.CustomRuleInput
		err  error
	}{
		{"both", routing.CustomRuleInput{OutboundGroupID: &id, OutboundLiteral: "DIRECT"}, routing.ErrInvalidOutbound},
		{"neither", routing.CustomRuleInput{}, routing.ErrInvalidOutbound},
		{"group only", routing.CustomRuleInput{OutboundGroupID: &id}, nil},
		{"literal only", routing.CustomRuleInput{OutboundLiteral: "DIRECT"}, nil},
	}
	for _, tc := range cases {
		got := tc.in.Validate()
		if !errors.Is(got, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.err)
		}
	}
}

func TestDeleteGroup_SystemImmutable(t *testing.T) {
	db := setupTestDB(t) // reused from builder_test.go (same routing_test package)
	if err := routing.DeleteGroup(context.Background(), db, 1); !errors.Is(err, routing.ErrSystemImmutable) {
		t.Errorf("want ErrSystemImmutable, got %v", err)
	}
}
