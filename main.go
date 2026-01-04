package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Prateek-Gupta001/AI_Gateway/api"
	"github.com/Prateek-Gupta001/AI_Gateway/cache"
	"github.com/Prateek-Gupta001/AI_Gateway/embed"
	"github.com/Prateek-Gupta001/AI_Gateway/llm"
	"github.com/Prateek-Gupta001/AI_Gateway/store"
	"github.com/joho/godotenv"
)

func main() {
	opts := returnOpts()
	ctx := context.Background()
	err := godotenv.Load()
	if err != nil {
		slog.Error("got this error while trying to load a dotenv file", "error", err)
	}
	slog.Info(os.Getenv("OPENAI_API_KEY"))
	// USE NewTextHandler INSTEAD OF NewJSONHandler
	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	slog.SetDefault(logger)
	slog.Info("The logger has been intialised!")
	store, err := store.NewStorage()
	if err != nil {
		slog.Error("Got this error while trying to create a New Storage ", "error", err.Error())
		panic(err)
	}
	if err2 := store.Init(); err2 != nil {
		slog.Error("Got this error while trying to intialise the postgres db ", "error", err2.Error())
		panic(err2)
	}
	llm := llm.NewLLMStruct()
	cache := cache.NewQdrantCache()
	go cache.ReviseCache(ctx)
	embed := embed.NewEmbeddingService(2, 100)
	server := api.NewAIGateway(":9000", store, llm, cache, embed)
	slog.Info("Server is running on port 9000!")
	server.Run()
}

func returnOpts() *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true, // We keep this, but we will clean it up below
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// 1. Customize the TIME format (make it shorter)
			if a.Key == slog.TimeKey {
				// changing format to just HH:MM:SS
				return slog.String("time", a.Value.Time().Format("15:04:05"))
			}

			if a.Key == slog.SourceKey {
				source, _ := a.Value.Any().(*slog.Source)
				if source != nil {
					// Instead of the full path, just show "file.go:line"
					// much easier to read
					return slog.String("src", filepath.Base(source.File)+":"+strconv.Itoa(source.Line))
				}
			}
			return a
		},
	}

}
