package main

import (
	"log/slog"
	"time"

	"github.com/JoshPattman/jpf"
)

// ModelBuilder builds LLM models.
type ModelBuilder interface {
	// BuildCandidateReviewModel builds a model for candidate review, using the specified logger.
	BuildCandidateReviewModel(*slog.Logger) jpf.Model
	// UsageCounter returns the usage counter for this model builder.
	UsageCounter() *jpf.UsageCounter
}

// NewModelBuilder tries to create a new ModelBuilder with the specified API key.
// The model will use cache that is persisted to ./cache.gob and will limit maximum number of concurrent connections.
func NewModelBuilder(apiKey string, apiURL string, modelName string, maxConcurrency int) (ModelBuilder, error) {
	cache, err := jpf.NewFilePersistCache("./cache.gob")
	if err != nil {
		return nil, err
	}
	return &simpleModelBuilder{
		apiKey:       apiKey,
		apiUrl:       apiURL,
		modelName:    modelName,
		concLimiter:  jpf.NewMaxConcurrentLimiter(maxConcurrency),
		cache:        cache,
		usageCounter: jpf.NewUsageCounter(),
	}, nil
}

type simpleModelBuilder struct {
	apiKey       string
	apiUrl       string
	modelName    string
	concLimiter  jpf.ConcurrentLimiter
	cache        jpf.ModelResponseCache
	usageCounter *jpf.UsageCounter
}

func (mb *simpleModelBuilder) BuildCandidateReviewModel(logger *slog.Logger) jpf.Model {
	model := jpf.NewOpenAIModel(mb.apiKey, mb.modelName, jpf.WithTemperature{X: 0}, jpf.WithURL{X: mb.apiUrl})
	model = jpf.NewLoggingModel(model, jpf.NewSlogModelLogger(logger.Info, false))
	model = jpf.NewRetryModel(model, 8, jpf.WithDelay{X: time.Second * 5})
	model = jpf.NewConcurrentLimitedModel(model, mb.concLimiter)
	model = jpf.NewCachedModel(model, mb.cache)
	model = jpf.NewUsageCountingModel(model, mb.usageCounter)
	return model
}

func (mb *simpleModelBuilder) UsageCounter() *jpf.UsageCounter {
	return mb.usageCounter
}
