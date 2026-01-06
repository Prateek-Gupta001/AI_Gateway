package embed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Prateek-Gupta001/AI_Gateway/types"
	"github.com/nlpodyssey/cybertron/pkg/models/bert"
	"github.com/nlpodyssey/cybertron/pkg/tasks"
	"github.com/nlpodyssey/cybertron/pkg/tasks/textencoding"
)

type Embed interface {
	SubmitJob(context.Context, string, chan types.EmbeddingResult)
}

type EmbeddingService struct {
	JobQueue chan types.EmbeddingJob
}

func NewEmbeddingService(numWorkers int, queueLen int) *EmbeddingService {
	jobQueue := make(chan types.EmbeddingJob, queueLen)
	for i := 0; i < numWorkers; i++ {
		go Worker(i, jobQueue)
	}
	return &EmbeddingService{
		JobQueue: jobQueue,
	}
}

func Worker(id int, jobQueue chan types.EmbeddingJob) {

	modelsDir := "models"
	modelName := textencoding.DefaultModel

	m, err := tasks.LoadModelForTextEncoding(&tasks.Config{ModelsDir: modelsDir, ModelName: modelName})
	if err != nil {
		slog.Error("error while loading the model", "error", err)
		fmt.Println(err)
	}
	slog.Info("Worker loaded with model ready to create embeddings!", "id", id)
	for job := range jobQueue {
		slog.Info("Worker got a job!", "id", id)
		start := time.Now()
		if job.Ctx.Err() != nil {
			continue
		}
		slog.Info("creating embedding", "query", job.Input)
		result, err := m.Encode(context.Background(), job.Input, int(bert.MeanPooling))
		slog.Info("Vector Preview", "v[0:5]", len(result.Vector.Data().F32()))
		if err != nil {
			job.ResultChan <- types.EmbeddingResult{
				Embedding_Result: types.Embedding{},
				Err:              err,
			}
			continue
		}
		n_result := types.EmbeddingResult{
			Embedding_Result: result.Vector.Data().F32(),
			Query:            job.Input,
			Err:              nil,
		}
		slog.Info("Work is almost complete! sending the results to the channel!", "id", id)
		end := time.Since(start)
		select {
		case <-job.Ctx.Done():
			slog.Info("Worker did its job but timeout happened!", "id", id, "time_taken", end.String())
			continue
		case job.ResultChan <- n_result:
			slog.Info("Worker did its job and sent the result to the channel!", "id", id, "time_taken", end.String())
			continue
		}

	}
}

func (s *EmbeddingService) SubmitJob(Ctx context.Context, Input string, ResultChan chan types.EmbeddingResult) {
	Job := types.EmbeddingJob{
		Ctx:        Ctx,
		Input:      Input,
		ResultChan: ResultChan,
	}
	select {
	case s.JobQueue <- Job:

	case <-Ctx.Done():

	}
}
