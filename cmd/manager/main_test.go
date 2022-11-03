package main

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetOptions(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	defaults := DefaultOptions()
	cases := []struct {
		Name            string
		Args            []string
		ExpectedOptions Options
	}{
		{"defaults", []string{}, defaults},
		{"withWebhookPort", []string{"-webhook-port=8000"},
			Options{
				metricsAddr:          defaults.metricsAddr,
				webhookPort:          8000,
				enableLeaderElection: defaults.enableLeaderElection,
			}},
		{"withMetricsAddr", []string{"-metrics-addr=:9090"},
			Options{
				metricsAddr:          ":9090",
				webhookPort:          defaults.webhookPort,
				enableLeaderElection: defaults.enableLeaderElection,
			}},
		{"withEnableLeaderElection", []string{"-leader-elect=true"},
			Options{
				metricsAddr:          defaults.metricsAddr,
				webhookPort:          defaults.webhookPort,
				enableLeaderElection: true,
			}},
		{"withSeveral", []string{"-webhook-port=8000", "-leader-elect=true"},
			Options{
				metricsAddr:          defaults.metricsAddr,
				webhookPort:          8000,
				enableLeaderElection: true,
			}},
		{"withAll", []string{"-metrics-addr=:9090", "-webhook-port=8000", "-leader-elect=true"},
			Options{
				metricsAddr:          ":9090",
				webhookPort:          8000,
				enableLeaderElection: true,
			}},
	}

	for _, tc := range cases {
		flag.CommandLine = flag.NewFlagSet(tc.Name, flag.ExitOnError)
		os.Args = append([]string{tc.Name}, tc.Args...)
		assert.Equal(t, tc.ExpectedOptions, GetOptions())
	}
}
