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
}

// NewModelBuilder tries to create a new ModelBuilder with the specified API key.
// The model will use cache that is persisted to ./cache.gob and will limit maximum number of concurrent connections.
func NewModelBuilder(apiKey string, maxConcurrency int) (ModelBuilder, error) {
	cache, err := jpf.NewFilePersistCache("./cache.gob")
	if err != nil {
		return nil, err
	}
	return &simpleModelBuilder{
		apiKey:      apiKey,
		modelName:   "gpt-4.1",
		concLimiter: jpf.NewMaxConcurrentLimiter(maxConcurrency),
		cache:       cache,
	}, nil
}

type simpleModelBuilder struct {
	apiKey      string
	modelName   string
	concLimiter jpf.ConcurrentLimiter
	cache       jpf.ModelResponseCache
}

func (mb *simpleModelBuilder) BuildCandidateReviewModel(logger *slog.Logger) jpf.Model {
	model := jpf.NewOpenAIModel(mb.apiKey, mb.modelName, jpf.WithTemperature{X: 0})
	model = jpf.NewLoggingModel(model, jpf.NewSlogModelLogger(logger.Info, false))
	model = jpf.NewRetryModel(model, 8, jpf.WithDelay{X: time.Second * 5})
	model = jpf.NewConcurrentLimitedModel(model, mb.concLimiter)
	model = jpf.NewCachedModel(model, mb.cache)
	return model
}
