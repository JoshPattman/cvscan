package app

import (
	"bytes"
	"cvscan/datamodels"
	"encoding/base64"
	"io"
	"log/slog"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
)

// Setup all of the handlers to their respective endpoints
func (app *App) setupHandlers(r *gin.Engine) {
	r.GET("/", app.handlePage(app.homePageHandler, app.homePageTemplates))
	r.GET("/manage-cvs", app.handlePage(app.manageCVsHandler, app.manageCVsTemplates))
	r.POST("/hx/upload-doc", app.handlePage(app.uploadDocsHandler, app.uploadDocsTemplates))
}

func (app *App) homePageHandler(*gin.Context, *slog.Logger) (any, error) {
	return struct {
		Title string
	}{"Home"}, nil
}

func (app *App) homePageTemplates(*gin.Context, *slog.Logger) []string {
	return []string{"page", "home"}
}

type CVPageData struct {
	Title           string
	AllGroupOptions []string
	CVs             []datamodels.CV
}

func (pd CVPageData) TableRowDatas() []any {
	type rowData struct {
		CV              datamodels.CV
		AllGroupOptions []string
	}
	data := make([]any, 0)
	for _, cv := range pd.CVs {
		data = append(data, rowData{cv, pd.AllGroupOptions})
	}
	return data
}

func (app *App) manageCVsHandler(*gin.Context, *slog.Logger) (any, error) {
	cvs, err := app.storageManager.ListCVs()
	if err != nil {
		return nil, err
	}
	groupOptions, err := app.storageManager.ListGroups()
	if err != nil {
		return nil, err
	}
	slices.Sort(groupOptions)
	return CVPageData{
		Title:           "Manage CVs",
		AllGroupOptions: groupOptions,
		CVs:             cvs,
	}, nil
}

func (app *App) manageCVsTemplates(*gin.Context, *slog.Logger) []string {
	return []string{"page", "manage_cvs/*"}
}

func (app *App) uploadDocsHandler(ctx *gin.Context, logger *slog.Logger) (any, error) {
	// Get uploaded file
	file, err := ctx.FormFile("upload")
	if err != nil {
		return nil, err
	}

	// Open the uploaded file
	f, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read file content into bytes
	data := make([]byte, file.Size)
	_, err = f.Read(data)
	if err != nil {
		return nil, err
	}

	pdfReader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	textReader, err := pdfReader.GetPlainText()
	if err != nil {
		return nil, err
	}

	textBytes, err := io.ReadAll(textReader)
	if err != nil {
		return nil, err
	}

	dataEncoded := base64.StdEncoding.EncodeToString(data)

	// Store CV with actual PDF bytes
	err = app.storageManager.StoreCV(datamodels.CV{
		UUID:     uuid.New().String(),
		FileName: file.Filename,
		Text:     string(textBytes),
		RawPDF:   dataEncoded,
		Group:    "Main group",
	})
	if err != nil {
		return nil, err
	}

	// Return updated table
	return app.manageCVsHandler(ctx, logger)
}

func (app *App) uploadDocsTemplates(*gin.Context, *slog.Logger) []string {
	return []string{"manage_cvs/manage_cvs_table", "manage_cvs/*"}
}

type SubmitButtonData struct {
	Class   string
	Content string
}
