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
	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	slog.SetDefault(logger)
	slog.Info("The logger has been intialised!")
	store, err := store.NewStorage(2)
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
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String("time", a.Value.Time().Format("15:04:05"))
			}

			if a.Key == slog.SourceKey {
				source, _ := a.Value.Any().(*slog.Source)
				if source != nil {
					return slog.String("src", filepath.Base(source.File)+":"+strconv.Itoa(source.Line))
				}
			}
			return a
		},
	}

}
