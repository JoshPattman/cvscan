package datamodels

// A CV represents an imported CV that has been parsed into text.
type CV struct {
	UUID     string
	FileName string
	Text     string
	RawPDF   string
	Group    string
}
