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
	SumbitJob(context.Context, string, chan types.EmbeddingResult)
}

type EmbeddingService struct {
	JobQueue chan types.EmbeddingJob
}

type EmbeddingWorkerChan struct {
	WorkerId int
	Error    error
}

func NewEmbeddingService(numWorkers int, queueLen int) *EmbeddingService {
	jobQueue := make(chan types.EmbeddingJob, queueLen)
	ErrorChan := make(chan EmbeddingWorkerChan, numWorkers)
	for i := 0; i < numWorkers; i++ {
		go Worker(i, jobQueue, ErrorChan)
	}
	return &EmbeddingService{
		JobQueue: jobQueue,
	}
}

func Worker(id int, jobQueue chan types.EmbeddingJob, ErrorChan chan EmbeddingWorkerChan) {

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

func (s *EmbeddingService) SumbitJob(Ctx context.Context, Input string, ResultChan chan types.EmbeddingResult) {
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

// import (
// 	"context"
// 	"log/slog"
// 	"time"

// 	"github.com/Prateek-Gupta001/AI_Gateway/types"
// 	"github.com/anush008/fastembed-go"
// )

// type Embed interface {
// 	SumbitJob(context.Context, string, chan types.EmbeddingResult)
// }

// type EmbeddingService struct {
// 	JobQueue chan types.EmbeddingJob
// }

// type EmbeddingWorkerChan struct {
// 	WorkerId int
// 	Error    error
// }

// func NewEmbeddingService(numWorkers int, queueLen int) *EmbeddingService {
// 	jobQueue := make(chan types.EmbeddingJob, queueLen)
// 	ErrorChan := make(chan EmbeddingWorkerChan, numWorkers)
// 	for i := 0; i < numWorkers; i++ {
// 		go Worker(i, jobQueue, ErrorChan)
// 	}
// 	return &EmbeddingService{
// 		JobQueue: jobQueue,
// 	}
// }

// func Worker(id int, jobQueue chan types.EmbeddingJob, ErrorChan chan EmbeddingWorkerChan) {

// 	options := &fastembed.InitOptions{
// 		Model: fastembed.BGESmallEN,
// 	}
// 	model, err := fastembed.NewFlagEmbedding(options)
// 	if err != nil {
// 		slog.Error("failed to load model:", "error", err, "id", id)
// 		ErrorChan <- EmbeddingWorkerChan{
// 			WorkerId: id,
// 			Error:    err,
// 		}
// 		return
// 	}
// 	slog.Info("Worker loaded with model ready to create embeddings!", "id", id)
// 	for job := range jobQueue {
// 		slog.Info("Worker got a job!", "id", id)
// 		start := time.Now()
// 		if job.Ctx.Err() != nil {
// 			continue
// 		}
// 		slog.Info("creating embedding", "query", job.Input)
// 		queryWithInstruction := "Represent this sentence for searching relevant passages: " + job.Input
// 		embeddings, err := model.PassageEmbed([]string{queryWithInstruction}, 1)
// 		slog.Info("Vector Preview", "v[0:5]", embeddings[0][:5])
// 		if err != nil {
// 			job.ResultChan <- types.EmbeddingResult{
// 				Embedding_Result: types.Embedding{},
// 				Err:              err,
// 			}
// 			continue
// 		}
// 		result := types.EmbeddingResult{
// 			Embedding_Result: embeddings,
// 			Query:            job.Input,
// 			Err:              nil,
// 		}
// 		slog.Info("Work is almost complete! sending the results to the channel!", "id", id)
// 		end := time.Since(start)
// 		select {
// 		case <-job.Ctx.Done():
// 			slog.Info("Worker did its job but timeout happened!", "id", id, "time_taken", end.String())
// 			continue
// 		case job.ResultChan <- result:
// 			slog.Info("Worker did its job and sent the result to the channel!", "id", id, "time_taken", end.String())
// 			continue
// 		}
// 	}
// }

// func (s *EmbeddingService) SumbitJob(Ctx context.Context, Input string, ResultChan chan types.EmbeddingResult) {
// 	Job := types.EmbeddingJob{
// 		Ctx:        Ctx,
// 		Input:      Input,
// 		ResultChan: ResultChan,
// 	}
// 	select {
// 	case s.JobQueue <- Job:

// 	case <-Ctx.Done():

// 	}
// }

// // func (s *EmbeddingService) MonitorWorker()

// // // type EmbeddingModel struct {
// // // 	model *fastembed.FlagEmbedding
// // // }

// // func NewEmbeddingModel() *EmbeddingModel {
// // 	options := &fastembed.InitOptions{
// // 		Model: fastembed.BGEBaseEN,
// // 	}
// // 	model, err := fastembed.NewFlagEmbedding(options)
// // 	if err != nil {
// // 		// You MUST stop here. You cannot return a nil model.
// // 		panic(fmt.Errorf("failed to load model: %w", err))
// // 	}
// // 	return &EmbeddingModel{
// // 		model: model,
// // 	}
// // }

// // func (e *EmbeddingModel) Close() {
// // 	e.model.Destroy()
// // }

// // func (e *EmbeddingModel) CreateQueryEmbedding(ctx context.Context, userQuery string, embeddingChan chan types.EmbeddingResult) {
// // 	if ctx.Err() != nil {
// // 		return
// // 	}
// // 	slog.Info("CreateQueryEmbedding was called here ", "query", userQuery)
// // 	slog.Info("model ", "model", *e.model)
// // 	//the error is coming over here
// // 	embeddings, err := (*e.model).PassageEmbed([]string{userQuery}, 1)
// // 	if err != nil {
// // 		slog.Error("Got this error right here while trying to createQuery Embedding!", "error", err)
// // 		embeddingChan <- types.EmbeddingResult{
// // 			Embedding_Result: nil,
// // 			Err:              err,
// // 		}
// // 		return
// // 	}
// // 	select {
// // 	case <-ctx.Done():
// // 		return
// // 	case embeddingChan <- types.EmbeddingResult{
// // 		Embedding_Result: embeddings,
// // 		Err:              nil,
// // 	}:
// // 		return
// // 	}
// // }
