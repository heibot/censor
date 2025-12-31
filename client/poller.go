package client

import (
	"context"
	"log"
	"sync"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/providers"
)

// PollerConfig configures the async task poller.
type PollerConfig struct {
	// PollInterval is how often to poll for pending tasks.
	PollInterval time.Duration

	// BatchSize is the maximum number of tasks to poll per provider per cycle.
	BatchSize int

	// Workers is the number of concurrent workers per provider.
	Workers int

	// Providers is the list of provider names to poll.
	Providers []string
}

// DefaultPollerConfig returns the default poller configuration.
func DefaultPollerConfig() PollerConfig {
	return PollerConfig{
		PollInterval: 30 * time.Second,
		BatchSize:    50,
		Workers:      3,
	}
}

// Poller polls for pending async tasks and processes them.
type Poller struct {
	client *Client
	config PollerConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// logger can be customized
	logger Logger
}

// Logger interface for logging.
type Logger interface {
	Printf(format string, v ...any)
}

// defaultLogger wraps standard log.
type defaultLogger struct{}

func (defaultLogger) Printf(format string, v ...any) {
	log.Printf(format, v...)
}

// NewPoller creates a new async task poller.
func NewPoller(client *Client, config PollerConfig) *Poller {
	if config.PollInterval == 0 {
		config.PollInterval = 30 * time.Second
	}
	if config.BatchSize == 0 {
		config.BatchSize = 50
	}
	if config.Workers == 0 {
		config.Workers = 3
	}

	// Default to all configured providers if not specified
	if len(config.Providers) == 0 {
		for name := range client.pipeline.providers {
			config.Providers = append(config.Providers, name)
		}
	}

	return &Poller{
		client: client,
		config: config,
		logger: defaultLogger{},
	}
}

// SetLogger sets a custom logger.
func (p *Poller) SetLogger(logger Logger) {
	p.logger = logger
}

// Start starts the poller.
func (p *Poller) Start(ctx context.Context) {
	p.ctx, p.cancel = context.WithCancel(ctx)

	for _, providerName := range p.config.Providers {
		// Check if provider supports async
		provider, ok := p.client.pipeline.providers[providerName]
		if !ok {
			continue
		}

		supportsAsync := false
		for _, cap := range provider.Capabilities() {
			for _, mode := range cap.Modes {
				if mode == providers.ModeAsync {
					supportsAsync = true
					break
				}
			}
			if supportsAsync {
				break
			}
		}

		if !supportsAsync {
			continue
		}

		p.wg.Add(1)
		go p.pollProvider(providerName)
	}

	p.logger.Printf("[Poller] Started polling for %d providers", len(p.config.Providers))
}

// Stop stops the poller and waits for all workers to finish.
func (p *Poller) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	p.logger.Printf("[Poller] Stopped")
}

// pollProvider polls a single provider for pending tasks.
func (p *Poller) pollProvider(providerName string) {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	// Initial poll
	p.pollOnce(providerName)

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.pollOnce(providerName)
		}
	}
}

// pollOnce performs a single poll cycle for a provider.
func (p *Poller) pollOnce(providerName string) {
	tasks, err := p.client.store.ListPendingAsyncTasks(p.ctx, providerName, p.config.BatchSize)
	if err != nil {
		p.logger.Printf("[Poller] Error listing pending tasks for %s: %v", providerName, err)
		return
	}

	if len(tasks) == 0 {
		return
	}

	p.logger.Printf("[Poller] Found %d pending tasks for %s", len(tasks), providerName)

	// Process tasks with worker pool
	taskChan := make(chan censor.PendingTask, len(tasks))
	for _, task := range tasks {
		taskChan <- task
	}
	close(taskChan)

	var wg sync.WaitGroup
	for i := 0; i < p.config.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskChan {
				p.processTask(task)
			}
		}()
	}
	wg.Wait()
}

// processTask processes a single pending task.
func (p *Poller) processTask(task censor.PendingTask) {
	provider, ok := p.client.pipeline.providers[task.Provider]
	if !ok {
		p.logger.Printf("[Poller] Provider %s not found for task %s", task.Provider, task.ProviderTaskID)
		return
	}

	// Query provider for task status
	resp, err := provider.Query(p.ctx, task.RemoteTaskID)
	if err != nil {
		p.logger.Printf("[Poller] Error querying task %s from %s: %v", task.RemoteTaskID, task.Provider, err)
		return
	}

	if !resp.Done {
		// Task still pending, skip
		return
	}

	// Task is done, process the result
	providerTask, err := p.client.store.GetProviderTask(p.ctx, task.ProviderTaskID)
	if err != nil {
		p.logger.Printf("[Poller] Error getting provider task %s: %v", task.ProviderTaskID, err)
		return
	}

	// Update provider task result
	if err := p.client.store.UpdateProviderTaskResult(p.ctx, task.ProviderTaskID, true, resp.Result, resp.Raw); err != nil {
		p.logger.Printf("[Poller] Error updating task result %s: %v", task.ProviderTaskID, err)
		return
	}

	// Process completion
	if err := p.client.processAsyncCompletion(p.ctx, providerTask, resp.Result); err != nil {
		p.logger.Printf("[Poller] Error processing completion for task %s: %v", task.ProviderTaskID, err)
		return
	}

	p.logger.Printf("[Poller] Successfully processed task %s from %s", task.RemoteTaskID, task.Provider)
}

// PollNow triggers an immediate poll for all providers.
// This can be called externally to force a poll cycle.
func (p *Poller) PollNow() {
	for _, providerName := range p.config.Providers {
		go p.pollOnce(providerName)
	}
}
