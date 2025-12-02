package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/JoshPattman/jpf"
)

// ModelBuilder builds LLM models.
type ModelBuilder interface {
	// BuildCandidateReviewModel builds a model for candidate review, using the specified logger.
	BuildCandidateReviewModel(*slog.Logger) jpf.Model
}

// NewModelBuilder tries to create a new ModelBuilder with the specified API key.
// The model will use cache that is persisted to ./cache.gob and will limit concurrency to 3.
func NewModelBuilder(apiKey string) (ModelBuilder, error) {
	cache, err := jpf.NewFilePersistCache("./cache.gob")
	if err != nil {
		return nil, err
	}
	return &simpleModelBuilder{
		apiKey:      apiKey,
		concLimiter: jpf.NewMaxConcurrentLimiter(3),
		cache:       cache,
	}, nil
}

type simpleModelBuilder struct {
	apiKey      string
	concLimiter jpf.ConcurrentLimiter
	cache       jpf.ModelResponseCache
}

func (mb *simpleModelBuilder) BuildCandidateReviewModel(logger *slog.Logger) jpf.Model {
	model := jpf.NewOpenAIModel(mb.apiKey, "gpt-4.1", jpf.WithTemperature{X: 0})
	model = jpf.NewLoggingModel(model, jpf.NewSlogModelLogger(logger.Info, false))
	model = jpf.NewRetryModel(model, 8, jpf.WithDelay{X: time.Second * 5})
	model = jpf.NewConcurrentLimitedModel(model, mb.concLimiter)
	model = jpf.NewCachedModel(model, mb.cache)
	return model
}

// CandidateQuestionResult represents the result of a single checklist question for a candidate.
type CandidateQuestionResult struct {
	probability float64
}

// IsTrue returns true if the candidate is likely to satisfy the checklist item.
func (c CandidateQuestionResult) IsTrue() bool {
	return c.probability > 0.5
}

// Inconsistency returns a measure of how inconsistent the model's answers were for this checklist item.
func (c CandidateQuestionResult) Inconsistency() float64 {
	return min(c.probability, 1-c.probability) * 2
}

// Probability returns the probability that the candidate satisfies the checklist item.
func (c CandidateQuestionResult) Probability() float64 {
	return c.probability
}

// Review the candidates' resumes against the checklist using the provided model builder and logger.
func ReviewCandidates(logger *slog.Logger, modelBuilder ModelBuilder, checklist map[string]string, resumes []string) ([]map[string]CandidateQuestionResult, error) {
	task := &candidateReviewTask{
		modelBuilder: modelBuilder,
		logger:       logger,
		checklist:    checklist,
		resumes:      resumes,
	}
	return task.Execute()
}

type candidateReviewTask struct {
	modelBuilder ModelBuilder
	logger       *slog.Logger
	checklist    map[string]string
	resumes      []string
	repeats      int
}

func (reviewer *candidateReviewTask) Execute() ([]map[string]CandidateQuestionResult, error) {
	results := make([]map[string]CandidateQuestionResult, len(reviewer.resumes))
	errs := make([]error, len(results))
	wg := &sync.WaitGroup{}
	wg.Add(len(results))
	for i := range reviewer.resumes {
		go func() {
			candidateLogger := reviewer.logger.With("resume", i)
			res, err := reviewer.reviewSingleCandidate(i)
			if err != nil {
				errs[i] = err
				candidateLogger.Error("Failed to review candidate", "err", err)
			} else {
				inconsistency := 0.0
				for _, v := range res {
					inconsistency += v.Inconsistency()
				}
				inconsistency /= float64(len(res))
				results[i] = res
				candidateLogger.Debug("Completed candidate review", "result", results[i])
				candidateLogger.Info("Completed candidate review", "inconsistency", math.Round(inconsistency*100)/100)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	errs = slices.DeleteFunc(errs, func(err error) bool { return err == nil })
	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}
	return results, nil
}

func (reviewer *candidateReviewTask) reviewSingleCandidate(candidateIndex int) (map[string]CandidateQuestionResult, error) {
	resultss := make([]map[string]bool, reviewer.repeats)
	errs := make([]error, reviewer.repeats)
	wg := &sync.WaitGroup{}
	wg.Add(reviewer.repeats)
	for i := range reviewer.repeats {
		go func() {
			defer wg.Done()
			repLogger := reviewer.logger.With("repeat", i)
			results, err := reviewer.reviewCandidateOnce(repLogger, candidateIndex, i)
			if err != nil {
				errs[i] = err
				repLogger.Error("failed to review candidate", "err", err)
			} else {
				resultss[i] = results
			}
		}()
	}
	wg.Wait()
	errs = slices.DeleteFunc(errs, func(err error) bool { return err == nil })
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	probs := make(map[string]CandidateQuestionResult)
	for _, results := range resultss {
		for k, v := range results {
			var delta float64
			if v {
				delta = 1
			}
			probs[k] = CandidateQuestionResult{
				probs[k].probability + delta,
			}
		}
	}
	for k, v := range probs {
		probs[k] = CandidateQuestionResult{
			v.probability / float64(reviewer.repeats),
		}
	}
	return probs, nil
}

type candidateReviewRequest struct {
	RepeatNumber int
	Checklist    map[string]string
	Resume       string
}

type candidateReviewResponse map[string]checklistItemResponse

type checklistItemResponse struct {
	Reasoning string `json:"reasoning"`
	Answer    bool   `json:"answer"`
}

type candidateReviewer jpf.MapFunc[candidateReviewRequest, candidateReviewResponse]

func (reviewer *candidateReviewTask) reviewCandidateOnce(logger *slog.Logger, candidateIndex int, repeatNumber int) (map[string]bool, error) {
	mf := buildReviewCandidateReviewMapFunc(reviewer.modelBuilder, logger)
	inputData := candidateReviewRequest{
		RepeatNumber: repeatNumber,
		Checklist:    reviewer.checklist,
		Resume:       reviewer.resumes[candidateIndex],
	}
	result, _, err := mf.Call(context.Background(), inputData)
	if err != nil {
		return nil, err
	}
	answers := make(map[string]bool)
	for key, resp := range result {
		answers[key] = resp.Answer
	}
	return answers, nil
}

// Build a mapfunc (a typed LLM call with retry logic) for reviewing a candidate.
func buildReviewCandidateReviewMapFunc(modelBuilder ModelBuilder, logger *slog.Logger) candidateReviewer {
	enc := jpf.NewTemplateMessageEncoder[candidateReviewRequest](
		"",
		simpleCandidateReviewTemplate,
	)
	dec := jpf.NewJsonResponseDecoder[candidateReviewRequest, candidateReviewResponse]()
	dec = jpf.NewValidatingResponseDecoder(
		dec,
		func(input candidateReviewRequest, response candidateReviewResponse) error {
			missingKeys := make([]string, 0)
			for k := range input.Checklist {
				if _, ok := response[k]; !ok {
					missingKeys = append(missingKeys, k)
				}
			}
			if len(missingKeys) > 0 {
				return fmt.Errorf("missing the following question keys: %v", missingKeys)
			}
			return nil
		},
	)
	fed := jpf.NewRawMessageFeedbackGenerator()
	model := modelBuilder.BuildCandidateReviewModel(logger)
	return jpf.NewFeedbackMapFunc(enc, dec, fed, model, jpf.UserRole, 10)
}

const simpleCandidateReviewTemplate = `You are an expert candidate reviewer. Examine the resume carefully and evaluate every checklist item.

For each checklist entry, produce:
- "reasoning": your full internal reasoning and thought process leading to the answer  
- "answer": true or false

Return a single JSON object where each key matches the exact checklist key.

Checklist:
{{ range $k, $v := .Checklist }}
- {{$k}}: {{$v}}
{{ end }}

Resume:
{{ .Resume }}

{{ .RepeatNumber }}`
