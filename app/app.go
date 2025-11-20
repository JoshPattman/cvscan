package app

import (
	"cvscan/storage"
	"embed"
	"html/template"
	"log/slog"
	"os"

	"github.com/MatusOllah/slogcolor"
	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

//go:embed templates
var templatesFS embed.FS

// A function that handles a request and returns data for a template.
type PageDataHander func(ctx *gin.Context, logger *slog.Logger) (any, error)

// A function that returns the main and auxillary templates to be rendered.
type PageTemplateDefiner func(ctx *gin.Context, logger *slog.Logger) []string

// App collects all data for running the webserver.
type App struct {
	config         Config
	logger         *slog.Logger
	modelBuilder   CandidateReviewModelBuilder
	storageManager storage.StorageManager
}

// Create a new app.
func BuildApp(logLevel slog.Level) (*App, error) {
	opts := slogcolor.DefaultOptions
	opts.Level = logLevel
	opts.MsgColor = color.New(color.FgMagenta)
	opts.SrcFileMode = slogcolor.Nop
	logger := slog.New(slogcolor.NewHandler(os.Stderr, opts))

	logger.Info("Reading config")
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	logger.Info("Creating model builder")
	modelBuilder, err := BuildModelBuilder(cfg.APIKey)
	if err != nil {
		return nil, err
	}

	logger.Info("Setting up CV manager")
	cvm, err := storage.NewFileCVManager("./cv-storage")
	if err != nil {
		return nil, err
	}

	logger.Info("Server preparation succsessful")
	return &App{
		config:         cfg,
		logger:         logger,
		modelBuilder:   modelBuilder,
		storageManager: cvm,
	}, nil
}

// Run the app, returning a fatal error.
func (app *App) Run() error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	app.setupHandlers(r)

	app.logger.Info("Server starting")
	return r.Run(":8080")
}

// Create a handler that calls the data handler then renders the data using the templates
func (app *App) handlePage(dataHandler PageDataHander, templateDefiner PageTemplateDefiner) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		requestLogger := app.logger.With("txid", uuid.New().String())
		requestLogger.Info("Incoming request", "path", ctx.Request.URL.Path)
		templatesToParse := []string{}
		templates := templateDefiner(ctx, requestLogger)
		for _, t := range templates {
			templatesToParse = append(templatesToParse, "templates/"+t+".html")
		}

		tmpl, err := template.ParseFS(templatesFS, templatesToParse...)
		if err != nil {
			requestLogger.Error("Template parse failed", "templates", templates, "error", err)
			ctx.Status(500)
			return
		}

		data, err := dataHandler(ctx, requestLogger)
		if err != nil {
			requestLogger.Error("Page data handler failed", "templates", templates, "error", err)
			ctx.Status(500)
			return
		}

		if err := tmpl.ExecuteTemplate(ctx.Writer, templates[0], data); err != nil {
			requestLogger.Error("Template render failed", "templates", templates, "error", err)
			ctx.Status(500)
			return
		}
		requestLogger.Info("Finished request")
	}
}
