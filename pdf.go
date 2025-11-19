package main

import (
	"bytes"

	"github.com/ledongthuc/pdf"
)

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
