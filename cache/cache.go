package cache

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Prateek-Gupta001/AI_Gateway/types"
	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var Tracer = otel.Tracer("ai-gateway-service")

type Cache interface {
	ExistsInCache(ctx context.Context, Embedding types.Embedding, userQuery string) (types.CacheResponse, bool, error) //if found then "query answer", true, nil ..If not found then "", false, nil ..
	InsertIntoCache(ctx context.Context, Embedding types.Embedding, llmResStruct types.LLMResponse, userQuery string)  //LLMAnswer will be stored in qdrant metadata!
}

type QdrantCache struct {
	Client    *qdrant.Client
	Threshold float32
}

func NewQdrantCache() *QdrantCache {
	//intialise the qdrant client
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: "localhost",
		Port: 6334,
	})
	if err != nil {
		//maybe add features in some sort of a way
		slog.Error("Got this error while trying to intialise the qdrant cache!", "error", err)
		//don't panic .. just return a flag which tells you .. okay .. caching layer is not working .. proceed without it!
		//Graceful degradation!
		//Do This!!!
		panic(err)
	}
	exists, err1 := client.CollectionExists(context.Background(), "AI_Gateway_Cache_1")
	if err1 != nil {
		slog.Error("Got this error while checking if collection exists or not!")
	}
	if !exists {
		slog.Info("new collection being created!")
		err = client.CreateCollection(context.Background(), &qdrant.CreateCollection{
			CollectionName: "AI_Gateway_Cache_1",
			VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
				Size:     384,
				Distance: qdrant.Distance_Cosine,
			}),
		})
		if err != nil {
			slog.Error("Got this error while trying to create the collection", "error", err)
		}
		_, err := client.CreateFieldIndex(context.Background(), &qdrant.CreateFieldIndexCollection{
			CollectionName: "AI_Gateway_Cache_1",
			FieldName:      "TTL",
			FieldType:      qdrant.FieldType_FieldTypeDatetime.Enum(), // Explicitly tell Qdrant this is a Date
		})
		if err != nil {
			slog.Error("Got this error while creating the qdrant cache!", "error", err)
		}
	}
	return &QdrantCache{
		Client:    client,
		Threshold: 0.85,
	}
}

func (q *QdrantCache) ExistsInCache(ctx context.Context, Embedding types.Embedding, userQuery string) (types.CacheResponse, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ctx, span := Tracer.Start(ctx, "Qdrant.ExistsInCache")

	defer span.End()
	searchResult, err := q.Client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: "AI_Gateway_Cache_1",
		Query:          qdrant.NewQuery(Embedding...),
		WithPayload:    qdrant.NewWithPayload(true),
		ScoreThreshold: &q.Threshold,
	})
	if err != nil {
		slog.Info("Got this error while trying to find if it ExistsInCache", "error", err)
		return types.CacheResponse{}, false, err
	}
	slog.Info("These are the search results", "results", searchResult)
	for _, results := range searchResult {
		slog.Info("CACHE HIT! Found something in the cache!")
		x := results.Payload
		Res := GetCachedRes(x)
		return *Res, true, nil
	}
	slog.Info("Cache Miss!")
	return types.CacheResponse{}, false, nil
}

func GetCachedRes(x map[string]*qdrant.Value) *types.CacheResponse {
	Res := &types.CacheResponse{}
	Res.CachedAnswer = string(x["CachedAnswer"].GetStringValue())
	Res.CachedQuery = string(x["CachedQuery"].GetStringValue())
	Res.InputTokens = int(x["InputTokens"].GetIntegerValue())
	Res.OutputTokens = int(x["OutputTokens"].GetIntegerValue())
	return Res
}

func (q *QdrantCache) InsertIntoCache(ctx context.Context, Embedding types.Embedding, llmResStruct types.LLMResponse, userQuery string) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ctx, span := Tracer.Start(ctx, "Qdrant.InsertIntoCache")
	defer span.End()
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(userQuery)).String()
	operationInfo, err := q.Client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: "AI_Gateway_Cache_1",
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewIDUUID(id),
				Vectors: qdrant.NewVectors(Embedding...),
				Payload: qdrant.NewValueMap(map[string]any{
					"InputTokens":  llmResStruct.InputTokens,
					"OutputTokens": llmResStruct.OutputTokens,
					"CachedAnswer": llmResStruct.LLMRes.String(),
					"CachedQuery":  userQuery,
					"TTL":          time.Now().Add(24 * time.Hour).Format(time.RFC3339), //inside cache for a day
				}),
			},
		},
	})
	if err != nil {
		slog.Error("Got this error while trying to insert the query into the cache!", "error", err)
		return
	}
	slog.Info("Insertion into cache successful!", "operationInfo", operationInfo)
}

func (q *QdrantCache) ReviseCache(ctx context.Context) {
	//this function goes via the qdrant cache and removes those points/vectors that have exceeded their TTL.
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			slog.Info("Our daily cache cleanup has begun!")
			res, err := q.Client.Delete(context.Background(), &qdrant.DeletePoints{
				CollectionName: "AI_Gateway_Cache_1",
				Points: qdrant.NewPointsSelectorFilter(
					&qdrant.Filter{
						Must: []*qdrant.Condition{
							qdrant.NewDatetimeRange("TTL", &qdrant.DatetimeRange{
								Lte: timestamppb.New(time.Now()),
							}),
						},
					},
				),
			})
			if err != nil {
				slog.Error("Got this error while revising/clearing the cache ", "error", err)
			}
			fmt.Println("Periodic Cache cleanup was succesful .. here are the results", "results", res)
		case <-ctx.Done():
			slog.Info("Stopping cache cleanup job...")
			return
		}
	}
}
