package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/ledongthuc/pdf"
)

// GetTextFromPDFFile extracts and returns the text content from a PDF file.
func GetTextFromPDFFile(file string) (string, error) {
	f, r, err := pdf.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	buf.ReadFrom(b)
	return buf.String(), nil
}

// CandidateReport represents the report for a single candidate.
type CandidateReport struct {
	FileName   string
	FileLoc    string
	Checklist  map[string]CandidateQuestionResult
	FinalScore float64
}

// ReportMode defines the mode of report generation.
type ReportMode uint8

const (
	// Boolean mode outputs true/false for checklist items.
	Boolean ReportMode = iota
	// Probability mode outputs the probability scores for checklist items.
	Probability
	// Inconsistency mode outputs the inconsistency scores for checklist items.
	Inconsistency
)

// WriteTextFile writes the given content to a text file with the specified filename.
func WriteTextFile(filename string, content string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	if err != nil {
		return err
	}
	return nil
}

// WriteCandidateReportsAsCSVFile writes the candidate reports to a CSV file in the specified mode.
func WriteCandidateReportsAsCSVFile(filename string, reports []CandidateReport, mode ReportMode) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return WriteCandidateReportsAsCSV(f, reports, mode)
}

// WriteCandidateReportsAsCSV writes the candidate reports to the provided writer in CSV format.
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
				row = append(row, fmt.Sprintf("%.3f", r.Checklist[k].probability))
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
