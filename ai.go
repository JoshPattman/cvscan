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

type ModelBuilder interface {
	BuildCandidateReviewModel(*slog.Logger) jpf.Model
}

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

type CandidateReviewData struct {
	I         int
	Checklist map[string]string
	Resume    string
}

type CandidateReviewResponse map[string]checklistItemResponse

type CandidateReviewer jpf.MapFunc[CandidateReviewData, CandidateReviewResponse]

type CandidateQuestionResult struct {
	Probability float64
}

func (c CandidateQuestionResult) IsTrue() bool {
	return c.Probability > 0.5
}

func (c CandidateQuestionResult) Inconsistency() float64 {
	return min(c.Probability, 1-c.Probability) * 2
}

func ReviewCandidates(modelBuilder ModelBuilder, checklist map[string]string, resumes []string, logger *slog.Logger) ([]map[string]CandidateQuestionResult, error) {
	results := make([]map[string]CandidateQuestionResult, len(resumes))
	errs := make([]error, len(results))
	wg := &sync.WaitGroup{}
	wg.Add(len(results))
	for i := range resumes {
		go func() {
			candidateLogger := logger.With("resume", i)
			res, err := reviewSingleCandidate(modelBuilder, checklist, resumes[i], 10, candidateLogger)
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

func reviewSingleCandidate(modelBuilder ModelBuilder, checklist map[string]string, resume string, repeats int, logger *slog.Logger) (map[string]CandidateQuestionResult, error) {
	resultss := make([]map[string]bool, repeats)
	errs := make([]error, repeats)
	wg := &sync.WaitGroup{}
	wg.Add(repeats)
	for i := range repeats {
		go func() {
			defer wg.Done()
			repLogger := logger.With("repeat", i)
			results, err := reviewCandidateHelper(modelBuilder, checklist, resume, i, repLogger)
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
				probs[k].Probability + delta,
			}
		}
	}
	for k, v := range probs {
		probs[k] = CandidateQuestionResult{
			v.Probability / float64(repeats),
		}
	}
	return probs, nil
}

func reviewCandidateHelper(modelBuilder ModelBuilder, checklist map[string]string, resume string, i int, logger *slog.Logger) (map[string]bool, error) {
	mf := buildReviewCandidateReviewMapFunc(modelBuilder, logger)
	inputData := CandidateReviewData{
		I:         i,
		Checklist: checklist,
		Resume:    resume,
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
func buildReviewCandidateReviewMapFunc(modelBuilder ModelBuilder, logger *slog.Logger) CandidateReviewer {
	enc := jpf.NewTemplateMessageEncoder[CandidateReviewData](
		"",
		simpleCandidateReviewTemplate,
	)
	dec := jpf.NewJsonResponseDecoder[CandidateReviewData, CandidateReviewResponse]()
	dec = jpf.NewValidatingResponseDecoder(
		dec,
		func(input CandidateReviewData, response CandidateReviewResponse) error {
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

type checklistItemResponse struct {
	Reasoning string `json:"reasoning"`
	Answer    bool   `json:"answer"`
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

{{ .I }}`
