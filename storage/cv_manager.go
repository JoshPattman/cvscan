package storage

import (
	"cvscan/datamodels"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

var ErrCVNotFound = errors.New("could not find cv")

// CVDTO is used only for storage and JSON encoding/decoding
type CVDTO struct {
	UUID     string `json:"uuid"`
	FileName string `json:"file_name"`
	Text     string `json:"text"`
	RawPDF   string `json:"raw_pdf"`
	Group    string `json:"group"`
}

type CVManager interface {
	ListCVIDs() ([]string, error)
	ListCVs() ([]datamodels.CV, error)
	GetCV(id string) (datamodels.CV, error)
	StoreCV(cv datamodels.CV) error
	DeleteCV(id string) error
}

// fallback to implement (inneficiently potentially) listCVs
func listCVs(cvm CVManager) ([]datamodels.CV, error) {
	cvIDs, err := cvm.ListCVIDs()
	if err != nil {
		return nil, err
	}
	cvs := make([]datamodels.CV, len(cvIDs))
	for i, id := range cvIDs {
		cvs[i], err = cvm.GetCV(id)
		if err != nil {
			return nil, err
		}
	}
	return cvs, nil
}

type fileCVManager struct {
	dir string
}

func NewFileCVManager(folder string) (*fileCVManager, error) {
	if err := os.MkdirAll(folder, 0755); err != nil {
		return nil, err
	}
	return &fileCVManager{dir: folder}, nil
}

func (cvm *fileCVManager) cvPath(id string) string {
	return filepath.Join(cvm.dir, id+".json")
}

// ListCVIDs lists all CV IDs (filenames without .json)
func (cvm *fileCVManager) ListCVIDs() ([]string, error) {
	files, err := os.ReadDir(cvm.dir)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, f := range files {
		if !f.IsDir() && filepath.Ext(f.Name()) == ".json" {
			ids = append(ids, f.Name()[:len(f.Name())-len(".json")])
		}
	}
	return ids, nil
}

// GetCV loads a CV from disk
func (cvm *fileCVManager) GetCV(id string) (datamodels.CV, error) {
	path := cvm.cvPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return datamodels.CV{}, ErrCVNotFound
		}
		return datamodels.CV{}, err
	}
	var dto CVDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return datamodels.CV{}, err
	}
	return datamodels.CV(dto), nil
}

// StoreCV saves a CV to disk
func (cvm *fileCVManager) StoreCV(cv datamodels.CV) error {
	dto := CVDTO(cv)
	data, err := json.MarshalIndent(dto, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cvm.cvPath(cv.UUID), data, 0644)
}

// DeleteCV removes a CV file
func (cvm *fileCVManager) DeleteCV(id string) error {
	path := cvm.cvPath(id)
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrCVNotFound
		}
		return err
	}
	return nil
}

func (cvm *fileCVManager) ListCVs() ([]datamodels.CV, error) {
	return listCVs(cvm)
}

func (cvm *fileCVManager) ListGroups() ([]string, error) {
	return []string{"default", "group_1"}, nil
}
