package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/JoshPattman/jpf"
)

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
func ReviewCandidates(logger *slog.Logger, modelBuilder ModelBuilder, checklist map[string]string, resumes []string, numRepeats int) ([]map[string]CandidateQuestionResult, error) {
	if len(resumes) == 0 {
		logger.Info("No resumes provided for checklist, skipping")
		return []map[string]CandidateQuestionResult{}, nil
	}
	if len(checklist) == 0 {
		logger.Info("No questions provided for checklist, skipping")
		return make([]map[string]CandidateQuestionResult, len(resumes)), nil
	}
	task := &candidateReviewTask{
		modelBuilder: modelBuilder,
		logger:       logger,
		checklist:    checklist,
		resumes:      resumes,
		repeats:      numRepeats,
	}
	logger.Info(
		"Reviewing resumes",
		"num_resumes", len(resumes),
		"num_checklist", len(checklist),
		"num_repeats", task.repeats,
		"estimated_llm_calls", len(resumes)*task.repeats,
	)
	return task.execute()
}

type candidateReviewTask struct {
	modelBuilder ModelBuilder
	logger       *slog.Logger
	checklist    map[string]string
	resumes      []string
	repeats      int
}

func (reviewer *candidateReviewTask) execute() ([]map[string]CandidateQuestionResult, error) {
	reviewer.logger.Info("Beginning candidate reviews", "num_candidates", len(reviewer.resumes))
	return ParMapRange(
		len(reviewer.resumes),
		func(i int) (map[string]CandidateQuestionResult, error) {
			candidateLogger := reviewer.logger.With("resume", i)
			candidateLogger.Info("Begun candidate review")
			res, err := reviewer.reviewSingleCandidate(i)
			if err != nil {
				candidateLogger.Error("Failed to review candidate", "err", err)
			} else {
				inconsistency := 0.0
				for _, v := range res {
					inconsistency += v.Inconsistency()
				}
				inconsistency /= float64(len(res))
				candidateLogger.Debug("Completed candidate review", "result", res)
				candidateLogger.Info("Completed candidate review", "inconsistency", math.Round(inconsistency*100)/100)
			}
			return res, err
		},
	)
}

func (reviewer *candidateReviewTask) reviewSingleCandidate(candidateIndex int) (map[string]CandidateQuestionResult, error) {
	// In parallell, repeat the review several times.
	resultsPerRepeat, err := ParMapRange(
		reviewer.repeats,
		func(i int) (map[string]bool, error) {
			repLogger := reviewer.logger.With("repeat", i)
			return reviewer.reviewCandidateOnce(repLogger, candidateIndex, i)
		},
	)
	if err != nil {
		return nil, err
	}
	// Aggregate the results.
	probs := make(map[string]CandidateQuestionResult)
	for _, results := range resultsPerRepeat {
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
