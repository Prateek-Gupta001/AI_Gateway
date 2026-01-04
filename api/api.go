package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Prateek-Gupta001/AI_Gateway/cache"
	"github.com/Prateek-Gupta001/AI_Gateway/embed"
	"github.com/Prateek-Gupta001/AI_Gateway/llm"
	"github.com/Prateek-Gupta001/AI_Gateway/store"
	"github.com/Prateek-Gupta001/AI_Gateway/types"
	"github.com/google/uuid"
)

type AIGateway struct {
	listenAddr string
	store      store.Storage
	llms       llm.LLMs
	cache      cache.Cache
	embed      embed.Embed
}

func NewAIGateway(addr string, store store.Storage, llm llm.LLMs, cache cache.Cache, embed embed.Embed) *AIGateway {
	return &AIGateway{
		listenAddr: addr,
		store:      store,
		llms:       llm,
		cache:      cache,
		embed:      embed,
	}
}

func (s *AIGateway) Run() {
	r := http.NewServeMux()
	r.HandleFunc("POST /chat", convertToHandleFunc(s.Chat))
	r.HandleFunc("GET /getRequests", convertToHandleFunc(s.GetAllRequests))
	if err := http.ListenAndServe(s.listenAddr, r); err != nil {
		slog.Info("Got this error while trying to run the server ", "error", err)
		panic(err)
	}
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

func (s *AIGateway) Chat(w http.ResponseWriter, r *http.Request) error {
	slog.Info("---------------------------------------NEW REQUEST---------------------------------------")
	start := time.Now()
	var req = &types.RequestStruct{}
	var request types.Request //this is the object that will be inserted in the db!
	request.Id = uuid.NewString()
	embedCtx, embedCancel := context.WithTimeout(r.Context(), time.Millisecond*200)
	embedGenCtx, embedGenCtxCancel := context.WithTimeout(context.Background(), time.Millisecond*7000)
	defer embedCancel()
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		slog.Info("Got this error while trying to decode the request struct ", "error", err)
		return err
	}
	embeddingChan := make(chan types.EmbeddingResult, 1)
	lastSlice := req.Messages[len(req.Messages)-1]
	if lastSlice.Role != "user" {
		http.Error(w, "The last role in the messages array cannot be either system or assistant!", http.StatusBadRequest)
		return fmt.Errorf("request has last role other than user")
	}
	userQuery := lastSlice.Content
	dynamic := checkTimeSensitivity(userQuery)
	slog.Info("is query dynamic?", "dynamic", dynamic)
	lenghtOfMsg := len(req.Messages)
	if !dynamic && lenghtOfMsg == 1 {
		go s.embed.SumbitJob(embedCtx, userQuery, embeddingChan)
		slog.Info("The query is not dynamic and its the first one! ..... being cached!")
		req.CacheFlag = true
	}
	var embedding types.Embedding
	request.UserId = req.UserId
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
			cacheRes, exists, err := s.cache.ExistsInCache(embedding, userQuery)
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
				err := s.store.InsertRequest(types.Request{
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
	err := s.llms.GenerateResponse(w, req.Messages, level, llmResStruct) //TODO: change this to level only ... this is just for testing!
	if err != nil {
		slog.Error("Got this error while trying to generate response from the LLM ", "error", err)
		return err
	}
	if err := s.store.IncrementUserTokens(req.UserId, llmResStruct.TotalTokens, llmResStruct.Level); err != nil {
		slog.Error("Got this error while trying to increment the user Tokens", "error", err)
	}
	slog.Info("REQEUST INFORMATION", "request.cachehit", request.CacheHit, "req.cacheflag", req.CacheFlag)
	if !request.CacheHit && req.CacheFlag {
		if embedding != nil {
			slog.Info("INSERTING INTO THE CACHE!")
			//embedding worker produced on time!
			go s.cache.InsertIntoCache(embedding, *llmResStruct, userQuery)
		} else {
			slog.Info("inside the else")
			go func() {
				defer embedGenCtxCancel()
				select {
				case result := <-embeddingChan:
					slog.Info("The worker did not create the embedding on time ... now lazy caching!")
					embedding = result.Embedding_Result
					s.cache.InsertIntoCache(embedding, *llmResStruct, userQuery)
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
	if err := s.store.InsertRequest(request); err != nil {
		slog.Info("Got this error while trying to insert the request in postgres db", "error", err, "request", request)
	}

	slog.Info("Query Answered!", "timeTaken", end)
	slog.Info("Response from the LLM was generated succesfully! At the end of request", "llmResStruct", llmResStruct)
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

func (s *AIGateway) GetAllRequests(w http.ResponseWriter, r *http.Request) error {
	requests, err := s.store.GetAllRequests()
	if err != nil {
		slog.Error("got this error", "error", err.Error())
		return err
	}
	WriteJSON(w, http.StatusOK, requests)
	return nil
}
