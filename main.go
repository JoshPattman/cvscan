package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/MatusOllah/slogcolor"
	"github.com/fatih/color"
)

func main() {
	numRepeats := flag.Int("r", 5, "number of repeats to run, higher is more accurate but costs more and is slower")
	maxConcurrentConnections := flag.Int("c", 3, "maximum number of concurrent connections to the LLM API, higher is faster but will rate limit more easily")
	apiKey := flag.String("k", "", "the openai api key, must always be specified")
	apiUrl := flag.String("u", "https://api.openai.com/v1/chat/completions", "the openai api url (or a url of any other openai-format api)")
	flag.Parse()

	tAllstart := time.Now()
	opts := slogcolor.DefaultOptions
	//opts.Level = slog.LevelDebug
	opts.MsgColor = color.New(color.FgMagenta)
	opts.SrcFileMode = slogcolor.Nop
	logger := slog.New(slogcolor.NewHandler(os.Stderr, opts))

	if *apiKey == "" {
		logger.Error("API key must be specified with -k")
		os.Exit(1)
	}

	logger.Info("Reading config")
	cfg, err := LoadConfig()
	if err != nil {
		logger.Error("Failed to load config", "err", err)
		os.Exit(1)
	}

	logger.Info("Reading PDFs")
	pdfs, err := readPDFsFromDir("./pdf")
	if err != nil {
		logger.Error("Failed to read PDFs", "err", err)
		os.Exit(1)
	}
	pdfNames := make([]string, 0, len(pdfs))
	pdfContents := make([]string, 0, len(pdfs))
	for k, v := range pdfs {
		pdfNames = append(pdfNames, k)
		pdfContents = append(pdfContents, v)
	}
	for i, path := range pdfNames {
		logger.Debug("Loaded PDF", "index", i, "path", path)
	}

	if err := os.MkdirAll("./result", os.ModePerm); err != nil {
		logger.Error("Failed to create result directory", "err", err)
		os.Exit(1)
	}

	if err := os.MkdirAll("./text", os.ModePerm); err != nil {
		logger.Error("Failed to create text directory", "err", err)
		os.Exit(1)
	}

	logger.Info("Saving text files")
	for k, v := range pdfs {
		name := filepath.Base(k)
		err = WriteTextFile(fmt.Sprintf("./text/%s.txt", name), v)
		if err != nil {
			logger.Error("Failed to write text file", "err", err, "file", name)
			os.Exit(1)
		}
	}

	logger.Info("Creating model builder")
	modelBuilder, err := NewModelBuilder(*apiKey, *apiUrl, *maxConcurrentConnections)
	if err != nil {
		logger.Error("Failed to create model builder", "err", err)
		os.Exit(1)
	}

	viewRunner := &viewRunner{
		logger:       logger,
		views:        cfg.Views,
		modelBuilder: modelBuilder,
		pdfNames:     pdfNames,
		pdfContents:  pdfContents,
		numRepeats:   *numRepeats,
	}
	err = ParMapDo(
		slices.Collect(maps.Keys(cfg.Views)),
		viewRunner.runView,
	)
	if err != nil {
		logger.Error("Failed to review candidates", "err", err)
		os.Exit(1)
	}

	logger.Info("Everything finished", "time_taken", time.Since(tAllstart))
}

type viewRunner struct {
	logger       *slog.Logger
	modelBuilder ModelBuilder
	views        map[string]ConfigView
	pdfNames     []string
	pdfContents  []string
	numRepeats   int
}

func (v *viewRunner) runView(viewName string) error {
	view := v.views[viewName]
	tstart := time.Now()
	checklist := checklistFromConfig(view)
	viewLogger := v.logger.With("view_name", viewName)
	result, err := ReviewCandidates(viewLogger, v.modelBuilder, checklist, v.pdfContents, v.numRepeats)
	if err != nil {
		return err
	}
	reports := make([]CandidateReport, len(result))
	for i := range result {
		finalScore := 0.0
		for key, cqc := range result[i] {
			if !cqc.IsTrue() {
				continue
			}
			finalScore += view.ScoreChecklist[key].Weight
		}
		reports[i] = CandidateReport{
			FileName:   filepath.Base(v.pdfNames[i]),
			FileLoc:    v.pdfNames[i],
			Checklist:  result[i],
			FinalScore: finalScore,
		}
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].FinalScore > reports[j].FinalScore {
			return true
		} else if reports[i].FinalScore < reports[j].FinalScore {
			return false
		} else {
			return reports[i].FileName < reports[j].FileName
		}
	})
	err = WriteCandidateReportsAsCSVFile(fmt.Sprintf("./result/report_%s.csv", viewName), reports, Boolean)
	if err != nil {
		return err
	}
	err = WriteCandidateReportsAsCSVFile(fmt.Sprintf("./result/probabilities_%s.csv", viewName), reports, Probability)
	if err != nil {
		return err
	}
	err = WriteCandidateReportsAsCSVFile(fmt.Sprintf("./result/inconsistency_%s.csv", viewName), reports, Inconsistency)
	if err != nil {
		return err
	}
	viewLogger.Info("Finished review", "time_taken", time.Since(tstart))
	return nil
}

func checklistFromConfig(cfg ConfigView) map[string]string {
	checklist := make(map[string]string)
	for key, val := range cfg.ScoreChecklist {
		checklist[key] = val.Question
	}
	return checklist
}

func listPDFs(dir string) ([]string, error) {
	var pdfFiles []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".pdf") {
			pdfFiles = append(pdfFiles, path)
		}
		return nil
	})

	return pdfFiles, err
}

func readPDFsFromDir(dir string) (map[string]string, error) {
	fileNames, err := listPDFs(dir)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, fn := range fileNames {
		text, err := GetTextFromPDFFile(fn)
		if err != nil {
			return nil, err
		}
		result[fn] = text
	}
	return result, nil
}
