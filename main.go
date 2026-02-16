package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/Prateek-Gupta001/AI_Gateway/api"
	"github.com/Prateek-Gupta001/AI_Gateway/cache"
	"github.com/Prateek-Gupta001/AI_Gateway/embed"
	"github.com/Prateek-Gupta001/AI_Gateway/llm"
	"github.com/Prateek-Gupta001/AI_Gateway/store"
	"github.com/Prateek-Gupta001/AI_Gateway/telemetry"
	"github.com/joho/godotenv"
)

func PrintBanner() {
	// ANSI Color Codes
	cyan := "\033[36m"
	green := "\033[32m"
	reset := "\033[0m"

	fmt.Println(cyan + `
    ___    ____  ________      __
   /   |  /  _/ / ____/ /___ _/ /____ _      ______ ___  __
  / /| |  / /  / / __/ / __ '/ __/ _ \ | /| / / __ '/ / / /
 / ___ |_/ /  / /_/ / / /_/ / /_/  __/ |/ |/ / /_/ / /_/ /
/_/  |_/___/  \____/_/\__,_/\__/\___/|__/|__/\__,_/\__, /
                                                  /____/` + reset)
	fmt.Println(green + " ⚡ High Performance AI Gateway v1.0" + reset)
	fmt.Println(cyan + " 🚀 Semantic Caching | 🛡️ Rate Limiting | 🧠 Smart Routing" + reset)
	fmt.Println(" -------------------------------------------------------")
}

func main() {
	PrintBanner()
	semantic_cache := true
	opts := returnOpts()
	ctx := context.Background()
	err := godotenv.Load()
	if err != nil {
		slog.Error("got this error while trying to load a dotenv file", "error", err)
	}
	shutdown, err := telemetry.InitTracer("ai-gateway")
	if err != nil {
		slog.Error("failed to init tracer", "error", err)
		os.Exit(1)
	}
	slog.Info("Telemetry has been intialised!")
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			slog.Error("failed to shutdown tracer", "error", err)
		}
	}()

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
	cache, err := cache.NewQdrantCache()
	if err != nil {
		slog.Info("Got an error while trying to setup the qdrant cache", "error", err)
		semantic_cache = false
	}
	go cache.ReviseCache(ctx)
	embed, err := embed.NewEmbeddingService("localhost:50051")
	if err != nil {
		slog.Info("Embedding Service Failed!")
		semantic_cache = false
	}
	slog.Info("Semantic Cache: ", "up: ", semantic_cache)
	server := api.NewAIGateway(":9000", store, llm, cache, embed, 1, semantic_cache)
	slog.Info("Server is running on port 9000!")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if err := server.Run(ctx, stop); err != nil {
		slog.Error("Got this error while running the server!", "error", err)
		panic(err)
	}
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
