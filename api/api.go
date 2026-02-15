package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	_ "net/http/pprof"

	"github.com/Prateek-Gupta001/AI_Gateway/cache"
	"github.com/Prateek-Gupta001/AI_Gateway/embed"
	"github.com/Prateek-Gupta001/AI_Gateway/llm"
	"github.com/Prateek-Gupta001/AI_Gateway/store"
	"github.com/Prateek-Gupta001/AI_Gateway/types"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type AIGateway struct {
	listenAddr        string
	store             store.Storage
	llms              llm.LLMs
	cache             cache.Cache
	embed             embed.Embed
	rateLimitDuration int
	RateLimiter       *RateLimiter
}

func NewAIGateway(addr string, store store.Storage, llm llm.LLMs, cache cache.Cache, embed embed.Embed, rateLimitDuration int) *AIGateway {
	return &AIGateway{
		listenAddr:        addr,
		store:             store,
		llms:              llm,
		cache:             cache,
		embed:             embed,
		rateLimitDuration: rateLimitDuration,
		RateLimiter: &RateLimiter{
			Users: make(map[string]time.Time),
		},
	}
}

func (s *AIGateway) Run(ctx context.Context, stop context.CancelFunc) (err error) {
	defer stop()
	go func() {
		slog.Info("Pprof attached: Pprof server running on localhost:6060")
		// "nil" tells it to use the DefaultServeMux where pprof registered itself
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			slog.Error("Pprof failed", "error", err)
		}
	}()
	r := s.newHTTPHandler()
	srv := &http.Server{
		Addr:         s.listenAddr,
		ReadTimeout:  time.Second * 5,
		WriteTimeout: time.Second * 30,
		Handler:      r,
	}
	srvErr := make(chan error, 1)
	go func() {
		slog.Info("Running HTTP server...")
		srvErr <- srv.ListenAndServe()
	}()
	select {
	case err := <-srvErr:
		return err
	case <-ctx.Done():
		stop()
	}
	slog.Info("Graceful Shutdown in progress!")
	timeCtx, _ := context.WithTimeout(context.Background(), time.Second*20)
	if err := srv.Shutdown(timeCtx); err != nil {
		slog.Info("got this error while doing graceful shutdown", "error", err)
		return err
	}

	slog.Info("Graceful shutdown successful!")
	return nil

}

func (s *AIGateway) newHTTPHandler() *http.ServeMux {
	r := http.NewServeMux()
	// r.HandleFunc("POST /chat", s.RateLimit(convertToHandleFunc((s.Chat))))
	r.HandleFunc("POST /chat", convertToHandleFunc((s.Chat)))
	r.HandleFunc("GET /stats", convertToHandleFunc(s.GetCostSaved))
	r.HandleFunc("GET /health", convertToHandleFunc(s.HealthCheck))
	return r
}

type apiFunc func(http.ResponseWriter, *http.Request) error

func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

func convertToHandleFunc(f apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r); err != nil {
			slog.Info("This error in convertToHandleFunc ", "error", err)
			WriteJSON(w, http.StatusInternalServerError, err)
		}
	}
}

type RateLimiter struct {
	Users map[string]time.Time
	mu    sync.Mutex
}

var Tracer = otel.Tracer("ai-gateway-service")

func (m *AIGateway) HealthCheck(w http.ResponseWriter, r *http.Request) error {
	slog.Info("Health check!")
	WriteJSON(w, http.StatusOK, "Server is healthy!")
	return nil
}

func (s *AIGateway) Chat(w http.ResponseWriter, r *http.Request) error {
	slog.Info("---------------------------------------NEW REQUEST---------------------------------------")
	start := time.Now()
	ctx, span := Tracer.Start(r.Context(), "Chat")

	defer span.End()
	var req = &types.RequestStruct{}
	userId := r.Header.Get("userId")
	span.SetAttributes(
		attribute.String("user_Id", userId),
	)
	slog.Info("userId is", "userId", userId)
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		slog.Info("Got this error while trying to decode the request struct ", "error", err)
		return err
	}
	lastSlice := req.Messages[len(req.Messages)-1]
	if lastSlice.Role != "user" {
		http.Error(w, "The last role in the messages array cannot be either system or assistant!", http.StatusBadRequest)
		return fmt.Errorf("request has last role other than user")
	}
	lenghtOfMsg := len(req.Messages)
	if lenghtOfMsg == 0 {
		http.Error(w, "No messages provided", http.StatusBadRequest)
		return fmt.Errorf("no messages provided")
	}
	var request types.Request //this is the object that will be inserted in the db!
	request.Id = uuid.NewString()
	embedCtx, embedCancel := context.WithTimeout(ctx, time.Millisecond*250)
	detachedCtx := context.WithoutCancel(r.Context())
	// STEP 2: Apply your specific 7-second logic to this valid, traced context
	embedGenCtx, embedGenCtxCancel := context.WithTimeout(detachedCtx, 7*time.Second)
	defer embedCancel()
	defer r.Body.Close()

	embeddingChan := make(chan types.EmbeddingResult, 1)

	userQuery := lastSlice.Content
	dynamic := checkTimeSensitivity(userQuery)
	slog.Info("is query dynamic?", "dynamic", dynamic)

	if !dynamic && lenghtOfMsg == 1 {
		go s.embed.SubmitJob(embedGenCtx, userQuery, embeddingChan)
		slog.Info("The query is not dynamic and its the first one! ..... being cached!")
		req.CacheFlag = true
	}
	var embedding types.Embedding
	request.UserId = userId
	request.Cacheable = req.CacheFlag
	slog.Info("cacheFlag", "cacheFlag", req.CacheFlag)
	if req.CacheFlag {
		slog.Info("inside the if")
		select {
		case <-embedCtx.Done():
			slog.Info("Embedding generation took more time than expected! Skipping embedding generation and moving onto llm response generation!")
		case result := <-embeddingChan:
			embedding = result.Embedding_Result
			slog.Info("embedding generation was successful", "query", result.Query)
			cacheRes, exists, err := s.cache.ExistsInCache(ctx, embedding, userQuery)
			request.CacheHit = exists

			if err != nil {
				//exit the if/select block here and go onto checking the complexity of the query
				//TODO: decide if you actually wanna treat a query api error as a cache miss .. cuz that would/could lead to similar query being cached twice!
				request.CacheHit = false
			}
			if !exists {
				//exit the if/select block here and go onto checking the complexity of the query
				slog.Info("Cache miss hence setting cache hit to false! query will be cached for future use!")
				request.CacheHit = false
			}
			if exists {
				end2 := time.Since(start)
				store_ctx := context.WithValue(context.Background(), types.UserIdKey, userId)
				s.store.SubmitInsertRequest(store_ctx, types.Request{
					Id:           request.Id,
					Cacheable:    request.Cacheable,
					UserId:       request.UserId,
					LLMResponse:  cacheRes.CachedAnswer,
					UserQuery:    cacheRes.CachedQuery,
					InputTokens:  cacheRes.InputTokens,
					OutputTokens: cacheRes.OutputTokens,
					Time:         end2,
					Model:        "",
					CacheHit:     request.CacheHit,
					Level:        types.High, //defaulting to high on cached requests
				})
				if err != nil {
					slog.Error("Got this error while trying to insert a request in the database", "error", err.Error())
				}
				//need to improve the writeJson here to ensure that the frontend/client knows there was a cache hit here!
				//need to set the headers here as well .. I guess
				slog.Info("Writing to the frontend!")
				WriteJSON(w, http.StatusOK, cacheRes)
				return nil
			}
		}
	}
	level := checkComplexity(userQuery)
	slog.Info("checking the complexity of the userQuery!", "level", level)
	llmResStruct := &types.LLMResponse{}
	err := s.llms.GenerateResponse(ctx, w, req.Messages, level, llmResStruct) //TODO: change this to level only ... this is just for testing!
	if err != nil {
		slog.Error("Got this error while trying to generate response from the LLM ", "error", err)
		return err
	}
	store_ctx := context.WithValue(context.Background(), types.UserIdKey, userId)
	s.store.SubmitIncrementUserTokens(store_ctx, userId, llmResStruct.TotalTokens, llmResStruct.Level)
	slog.Info("REQEUST INFORMATION", "request.cachehit", request.CacheHit, "req.cacheflag", req.CacheFlag)
	cache_insert_ctx := context.WithoutCancel(ctx)
	if !request.CacheHit && req.CacheFlag {
		if embedding != nil {
			slog.Info("INSERTING INTO THE CACHE!")
			//embedding worker produced on time!
			go s.cache.InsertIntoCache(cache_insert_ctx, embedding, *llmResStruct, userQuery)
		} else {
			slog.Info("inside the else")
			go func() {
				defer embedGenCtxCancel()
				select {
				case result := <-embeddingChan:
					slog.Info("The worker did not create the embedding on time but in less than 7 seconds ... now lazy caching!")
					embedding = result.Embedding_Result
					s.cache.InsertIntoCache(cache_insert_ctx, embedding, *llmResStruct, userQuery)
				case <-embedGenCtx.Done():
					slog.Info("Embedding Generation was taking longer than 7 seconds... skipping caching even though cacheable and cache miss")
				}
			}()
		}
	}

	request.InputTokens = llmResStruct.InputTokens
	request.OutputTokens = llmResStruct.OutputTokens
	request.TotalToken = llmResStruct.TotalTokens
	request.Model = llmResStruct.Model
	request.Level = llmResStruct.Level
	request.LLMResponse = llmResStruct.LLMRes.String()
	request.UserQuery = userQuery
	end := time.Since(start)
	request.Time = end
	slog.Info("inserting this request into the database!", "request", request)
	insert_ctx := context.WithValue(context.Background(), types.UserIdKey, userId)
	s.store.SubmitInsertRequest(insert_ctx, request)

	slog.Info("Query Answered!", "timeTaken", end)
	slog.Info("Response from the LLM was generated succesfully! At the end of request", "llmResStruct", llmResStruct)
	span.SetAttributes(
		attribute.Bool("cachehit", request.CacheHit),
	)
	return nil
}

func checkTimeSensitivity(query string) bool {
	words := []string{"now", "today", "weather", "latest", "time", "today's", "current"}
	for _, value := range words {
		if strings.Contains(query, value) {
			return true
		}
	}
	return false
}

func checkComplexity(query string) types.Level {
	numWords := strings.Fields(query)
	if len(numWords) >= 10 {
		return types.High
	}
	return types.Easy
}

// func (s *AIGateway) GetAllRequests(w http.ResponseWriter, r *http.Request) error {
// 	requests, err := s.store.GetAllRequests()
// 	if err != nil {
// 		slog.Error("got this error", "error", err.Error())
// 		return err
// 	}
// 	WriteJSON(w, http.StatusOK, requests)
// 	return nil
// }

func (s *AIGateway) GetCostSaved(w http.ResponseWriter, r *http.Request) error {
	Analytics, err := s.store.GetAnalytics()
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, Analytics)
	return nil
}
