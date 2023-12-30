package agent

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestProcessCommands(t *testing.T) {
	testCases := []struct {
		name     string
		commands []ModelOp
	}{
		{
			name:     "EmptyCommands",
			commands: []ModelOp{},
		},
		{
			name: "InvalidModelSpecification",
			commands: []ModelOp{
				{ModelName: "invalidModel", Op: Add, Spec: nil},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger := zap.NewNop().Sugar()
			downloader := &Downloader{}
			puller := Puller{
				channelMap:  make(map[string]*ModelChannel),
				completions: make(chan *ModelOp, 4),
				opStats:     make(map[string]map[OpType]int),
				waitGroup:   WaitGroupWrapper{sync.WaitGroup{}},
				Downloader:  downloader,
				logger:      logger,
			}

			commands := make(chan ModelOp, len(tc.commands))
			for _, cmd := range tc.commands {
				commands <- cmd
			}
			close(commands)

			go puller.processCommands(commands)
			puller.waitGroup.wg.Wait()

			assert.Empty(t, puller.opStats)
		})
	}
}

func TestStartPullerAndProcessModels(t *testing.T) {
	testCases := []struct {
		name       string
		downloader *Downloader
		commands   chan ModelOp
		logger     *zap.SugaredLogger
	}{
		{
			name:       "ValidInputs",
			downloader: &Downloader{},
			commands:   make(chan ModelOp),
			logger:     zap.NewNop().Sugar(),
		},
		{
			name:       "NilDownloader",
			downloader: nil,
			commands:   make(chan ModelOp),
			logger:     zap.NewNop().Sugar(),
		},
		{
			name:       "NilCommands",
			downloader: &Downloader{},
			commands:   nil,
			logger:     zap.NewNop().Sugar(),
		},
		{
			name:       "NilLogger",
			downloader: &Downloader{},
			commands:   make(chan ModelOp),
			logger:     nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.commands != nil {
				defer close(tc.commands)
			}
			assert.NotPanics(t, func() {
				StartPullerAndProcessModels(tc.downloader, tc.commands, tc.logger)
			})
		})
	}
}
