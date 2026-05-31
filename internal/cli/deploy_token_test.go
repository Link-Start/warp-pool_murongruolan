package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
)

func TestDeployTokenStatus(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name string
		item config.DeployToken
		want string
	}{
		{
			name: "registered",
			item: config.DeployToken{Used: true, Registered: true},
			want: "registered",
		},
		{
			name: "used",
			item: config.DeployToken{Used: true},
			want: "used",
		},
		{
			name: "expired",
			item: config.DeployToken{ExpiresAt: now.Add(-time.Minute).Format(time.RFC3339)},
			want: "expired",
		},
		{
			name: "prepared",
			item: config.DeployToken{ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), Prepared: true},
			want: "prepared",
		},
		{
			name: "unused",
			item: config.DeployToken{ExpiresAt: now.Add(time.Minute).Format(time.RFC3339)},
			want: "unused",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deployTokenStatus(tc.item, now); got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestShortDeployToken(t *testing.T) {
	got := shortDeployToken("1234567890abcdef")
	if !strings.HasPrefix(got, "123456...") || !strings.HasSuffix(got, "abcdef") {
		t.Fatalf("unexpected short token: %s", got)
	}
}
