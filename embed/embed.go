package embed

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Prateek-Gupta001/AI_Gateway/types"
	"github.com/anush008/fastembed-go"
)

type Embed interface {
	CreateQueryEmbedding(context.Context, string, chan types.EmbeddingResult)
	Close()
}

type EmbeddingModel struct {
	model *fastembed.FlagEmbedding
}

func NewEmbeddingModel() *EmbeddingModel {
	options := &fastembed.InitOptions{
		Model: fastembed.BGEBaseEN,
	}
	model, err := fastembed.NewFlagEmbedding(options)
	if err != nil {
		// You MUST stop here. You cannot return a nil model.
		panic(fmt.Errorf("failed to load model: %w", err))
	}
	return &EmbeddingModel{
		model: model,
	}
}

func (e *EmbeddingModel) Close() {
	e.model.Destroy()
}

func (e *EmbeddingModel) CreateQueryEmbedding(ctx context.Context, userQuery string, embeddingChan chan types.EmbeddingResult) {
	if ctx.Err() != nil {
		return
	}
	slog.Info("CreateQueryEmbedding was called here ", "query", userQuery)
	slog.Info("model ", "model", *e.model)
	//the error is coming over here
	embeddings, err := (*e.model).PassageEmbed([]string{userQuery}, 1)
	if err != nil {
		slog.Error("Got this error right here while trying to createQuery Embedding!", "error", err)
		embeddingChan <- types.EmbeddingResult{
			Embedding_Result: nil,
			Err:              err,
		}
		return
	}
	select {
	case <-ctx.Done():
		return
	case embeddingChan <- types.EmbeddingResult{
		Embedding_Result: embeddings,
		Err:              nil,
	}:
		return

	}
}
