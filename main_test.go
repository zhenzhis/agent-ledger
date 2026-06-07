package main

import "testing"

func TestCLICommandRequiresWriteForNotifyDryRun(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "notify webhook sends", args: []string{"notify", "webhook"}, want: true},
		{name: "notify webhook dry run is read-only", args: []string{"notify", "webhook", "--dry-run"}, want: false},
		{name: "notify webhook dry run with filters is read-only", args: []string{"notify", "webhook", "--severity", "warning", "--dry-run"}, want: false},
		{name: "notify without subcommand remains write", args: []string{"notify"}, want: true},
		{name: "notify other subcommand remains write", args: []string{"notify", "other", "--dry-run"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cliCommandRequiresWrite(tc.args); got != tc.want {
				t.Fatalf("cliCommandRequiresWrite(%v)=%v want %v", tc.args, got, tc.want)
			}
		})
	}
}
