package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/JoshPattman/jpf"
)

type CandidateTextQuestionResult struct {
	Reasoning string
	Answer    string
}

func AnswerQuestionsForCandidates(logger *slog.Logger, modelBuilder ModelBuilder, questions map[string]string, resumes []string) ([]map[string]CandidateTextQuestionResult, error) {
	if len(resumes) == 0 {
		logger.Info("No resumes provided for question answering, skipping")
		return []map[string]CandidateTextQuestionResult{}, nil
	}
	if len(questions) == 0 {
		logger.Info("No questions provided for question answering, skipping")
		return make([]map[string]CandidateTextQuestionResult, len(resumes)), nil
	}
	task := &candidateQuestionsTask{
		logger:       logger,
		modelBuilder: modelBuilder,
		questions:    questions,
		resumes:      resumes,
	}
	logger.Info(
		"Answering questions",
		"num_resumes", len(resumes),
		"num_questions", len(questions),
		"estimated_llm_calls", len(resumes),
	)
	return task.execute()
}

type candidateQuestionsTask struct {
	logger       *slog.Logger
	modelBuilder ModelBuilder
	questions    map[string]string
	resumes      []string
}

func (task *candidateQuestionsTask) execute() ([]map[string]CandidateTextQuestionResult, error) {
	task.logger.Info("Beginning question answering", "num_candidates", len(task.resumes))
	return ParMapRange(
		len(task.resumes),
		func(i int) (map[string]CandidateTextQuestionResult, error) {
			candidateLogger := task.logger.With("resume", i)
			candidateLogger.Info("Begun question answering")
			res, err := task.qaSingleCandidate(i)
			if err != nil {
				candidateLogger.Error("Failed to answer questions for candidate", "err", err)
			} else {
				candidateLogger.Debug("Completed question answering", "result", res)
				candidateLogger.Info("Completed question answering")
			}
			return res, err
		},
	)
}

type candidateQuestionRequest struct {
	Resume    string
	Questions map[string]string
}

type candidateQuestionResponse struct {
	Reasoning string `json:"reasoning"`
	Answer    string `json:"answer"`
}

type candidateQuestionsResponse map[string]candidateQuestionResponse

type candidateQuestioner jpf.MapFunc[candidateQuestionRequest, candidateQuestionsResponse]

func (task *candidateQuestionsTask) qaSingleCandidate(candidateIndex int) (map[string]CandidateTextQuestionResult, error) {
	mf := buildQuestionCandidateMapFunc(task.modelBuilder, task.logger)
	req := candidateQuestionRequest{
		Resume:    task.resumes[candidateIndex],
		Questions: task.questions,
	}
	result, _, err := mf.Call(context.Background(), req)
	if err != nil {
		return nil, err
	}
	answers := make(map[string]CandidateTextQuestionResult)
	for k, v := range result {
		answers[k] = CandidateTextQuestionResult(v)
	}
	return answers, nil
}

func buildQuestionCandidateMapFunc(modelBuilder ModelBuilder, logger *slog.Logger) candidateQuestioner {
	enc := jpf.NewTemplateMessageEncoder[candidateQuestionRequest](
		"",
		simpleCandidateQuestionTemplate,
	)
	dec := jpf.NewJsonResponseDecoder[candidateQuestionRequest, candidateQuestionsResponse]()
	dec = jpf.NewValidatingResponseDecoder(
		dec,
		func(input candidateQuestionRequest, response candidateQuestionsResponse) error {
			missingKeys := make([]string, 0)
			for k := range input.Questions {
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

const simpleCandidateQuestionTemplate = `You are an expert candidate reviewer. Examine the resume carefully and evaluate every question item.

For each question entry, produce:
- "reasoning": your full internal reasoning and thought process leading to the answer  
- "answer": a string answer to the question (if not otherwise specified, this should be as concise as possible)

Return a single JSON object where each key matches the exact question key. Do not return extra keys, and make sure to answer all questions.

Questions:
{{ range $k, $v := .Questions }}
- {{$k}}: {{$v}}
{{ end }}

Resume:
{{ .Resume }}`
