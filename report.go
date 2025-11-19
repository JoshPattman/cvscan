package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
)

type CandidateReport struct {
	FileName   string
	FileLoc    string
	Checklist  map[string]CandidateQuestionResult
	FinalScore float64
}

type ReportMode uint8

const (
	Boolean ReportMode = iota
	Probability
	Inconsistency
)

func WriteCandidateReportsAsCSVFile(filename string, reports []CandidateReport, mode ReportMode) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return WriteCandidateReportsAsCSV(f, reports, mode)
}

func WriteCandidateReportsAsCSV(w io.Writer, reports []CandidateReport, mode ReportMode) error {
	cw := csv.NewWriter(w)

	// Collect all checklist keys
	keySet := make(map[string]struct{})
	for _, r := range reports {
		for k := range r.Checklist {
			keySet[k] = struct{}{}
		}
	}

	// Sort keys alphabetically
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build header
	header := []string{"FileName", "FileLoc"}
	header = append(header, keys...)
	header = append(header, "FinalScore")

	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, r := range reports {
		row := make([]string, 0, len(header))

		row = append(row, r.FileName, r.FileLoc)

		for _, k := range keys {
			switch mode {
			case Boolean:
				if r.Checklist[k].IsTrue() {
					row = append(row, "true")
				} else {
					row = append(row, "false")
				}
			case Probability:
				row = append(row, fmt.Sprintf("%.3f", r.Checklist[k].Probability))
			case Inconsistency:
				row = append(row, fmt.Sprintf("%.3f", r.Checklist[k].Inconsistency()))
			}
		}

		row = append(row, strconv.FormatFloat(r.FinalScore, 'f', -1, 64))

		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}

	cw.Flush()
	return cw.Error()
}
