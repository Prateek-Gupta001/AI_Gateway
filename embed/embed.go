package embed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	pb "github.com/Prateek-Gupta001/AI_Gateway/proto/embedding"
	"github.com/Prateek-Gupta001/AI_Gateway/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Embed interface {
	GenerateDenseEmbedding(query string, ctx context.Context, resultChan chan types.EmbeddingResult)
}

var Tracer = otel.Tracer("ai-gateway-service")

type EmbeddingClient struct {
	EmbedServiceUrl string
	conn            *grpc.ClientConn
	client          pb.EmbeddingServiceClient
}

func NewEmbeddingService(url string) (*EmbeddingClient, error) {
	// Create gRPC connection with proper options
	conn, err := grpc.NewClient(
		url,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	// Create the embedding service client
	client := pb.NewEmbeddingServiceClient(conn)

	return &EmbeddingClient{
		EmbedServiceUrl: url,
		conn:            conn,
		client:          client,
	}, nil
}

func (e *EmbeddingClient) GenerateDenseEmbedding(query string, ctx context.Context, result chan types.EmbeddingResult) {
	slog.Info("Got a dense embedding generation request!")
	ctx, span := Tracer.Start(ctx, "Dense Embedding Generation")
	defer span.End()
	span.SetAttributes(attribute.String("Query", query))
	// Validate input
	if query == "" {
		result <- types.EmbeddingResult{
			Err: fmt.Errorf("No query provided!"),
		}
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Create the request
	req := &pb.Query{
		Query: query,
	}

	// Make the gRPC call
	resp, err := e.client.CreateDenseEmbedding(ctx, req)
	if err != nil {
		result <- types.EmbeddingResult{
			Err: fmt.Errorf("failed to create dense embedding: %w", err),
		}
		return

	}

	// Convert protobuf response to types
	denseEmbedding := types.DenseEmbedding{
		Values: resp.Values,
	}

	select {
	case <-ctx.Done():
		result <- types.EmbeddingResult{
			Err: fmt.Errorf("Ctx expired!"),
		}

	default:
		result <- types.EmbeddingResult{
			Embedding_Result: &denseEmbedding,
			Query:            query,
			Err:              nil,
		}

	}
}
