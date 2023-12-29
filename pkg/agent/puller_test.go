package agent

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestEmptyCommands(t *testing.T) {
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

	// Create a test command channel with no commands
	commands := make(chan ModelOp)
	close(commands)

	go puller.processCommands(commands)
	puller.waitGroup.wg.Wait()

	assert.Empty(t, puller.opStats)
}

func TestInvalidModelSpecification(t *testing.T) {
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

	// Create a test command channel with an invalid model specification
	commands := make(chan ModelOp, 1)
	commands <- ModelOp{ModelName: "invalidModel", Op: Add, Spec: nil}
	close(commands)

	go puller.processCommands(commands)
	puller.waitGroup.wg.Wait()

	assert.Empty(t, puller.opStats)
}

func TestStartPullerAndProcessModels_ValidInputs(t *testing.T) {
	logger := zap.NewNop().Sugar()
	downloader := &Downloader{}
	commands := make(chan ModelOp)
	defer close(commands)

	assert.NotPanics(t, func() {
		StartPullerAndProcessModels(downloader, commands, logger)
	})
}

func TestStartPullerAndProcessModels_NilDownloader(t *testing.T) {
	logger := zap.NewNop().Sugar()
	commands := make(chan ModelOp)
	defer close(commands)

	assert.NotPanics(t, func() {
		StartPullerAndProcessModels(nil, commands, logger)
	})
}

func TestStartPullerAndProcessModels_NilCommands(t *testing.T) {
	logger := zap.NewNop().Sugar()
	downloader := &Downloader{}

	assert.NotPanics(t, func() {
		StartPullerAndProcessModels(downloader, nil, logger)
	})
}

func TestStartPullerAndProcessModels_NilLogger(t *testing.T) {
	downloader := &Downloader{}
	commands := make(chan ModelOp)
	defer close(commands)

	assert.NotPanics(t, func() {
		StartPullerAndProcessModels(downloader, commands, nil)
	})
}
