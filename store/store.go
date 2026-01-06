package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/lib/pq"

	"github.com/Prateek-Gupta001/AI_Gateway/types"
)

type Storage interface {
	SubmitInsertRequest(context.Context, types.Request)
	SubmitIncrementUserTokens(context.Context, string, int, types.Level)
	GetAnalytics() (types.AnalyticsResponse, error)
	GetAllRequests() ([]*types.Request, error)
}

type PostgresStore struct {
	db                 *sql.DB
	InsertRequestChan  chan types.InsertRequestPayload
	IncrementTokenChan chan types.IncTokenPayload
}

func (s *PostgresStore) StoreWorker(id int) {
	slog.Info("Starting StoreWorker", "id", id)
	for {
		select {
		case val := <-s.InsertRequestChan:
			err := s.InsertRequest(val.Ctx, val.Request)
			if err != nil {
				uid, _ := val.Ctx.Value(types.UserIdKey).(string)
				slog.Error("Got this error while trying to insert the request Id", "id", uid, "error", err)
			} //TODO: Add id (from context!)
		case val := <-s.IncrementTokenChan:
			err := s.IncrementUserTokens(val.Ctx, val.UserId, val.Tokens, val.Level)
			if err != nil {
				uid, _ := val.Ctx.Value(types.UserIdKey).(string)
				slog.Error("Got this error while trying to increment user tokens", "id", uid, "error", err)
			} //TODO: Add id (from context!)
		}
	}
}

func (s *PostgresStore) SubmitInsertRequest(ctx context.Context, request types.Request) {
	select {
	case s.InsertRequestChan <- types.InsertRequestPayload{
		Request: request,
		Ctx:     ctx,
	}:

	default:
		slog.Info("channel is full! dropping insert request to preserve latency!")
	}

}

func (s *PostgresStore) SubmitIncrementUserTokens(ctx context.Context, userId string, tokens int, level types.Level) {

	select {
	case s.IncrementTokenChan <- types.IncTokenPayload{
		UserId: userId,
		Tokens: tokens,
		Level:  level,
		Ctx:    ctx,
	}:

	default:
		slog.Info("channel is full! dropping increment tokens to preserve latency!")
	}
}

func NewStorage(numWorkers int) (*PostgresStore, error) {

	dbPassword := os.Getenv("DB_PASSWORD")
	connStr := fmt.Sprintf("host=127.0.0.1 port=5432 user=postgres dbname=postgres password=%s sslmode=disable", dbPassword)
	// ... rest of code
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		slog.Info("Got this error while trying to open a connection to the database ", "error", err)
		return nil, err
	}
	if err = db.Ping(); err != nil {
		slog.Info("Got this error while trying to ping the database ", "error", err)
		return nil, err
	}
	InsertRequestChan := make(chan types.InsertRequestPayload, 100)
	IncrementTokenChan := make(chan types.IncTokenPayload, 100)

	ps := &PostgresStore{
		db:                 db,
		InsertRequestChan:  InsertRequestChan,
		IncrementTokenChan: IncrementTokenChan,
	}
	for i := 0; i < numWorkers; i++ {
		go ps.StoreWorker(i)
	}
	return ps, nil
}

func (s *PostgresStore) Init() error {
	//user id is a random string
	query1 := `CREATE TABLE IF NOT EXISTS Account(
	user_id varchar(50) primary key,
	simple_tokens BIGINT NOT NULL default 0, 
	complex_tokens BIGINT NOT NULL default 0,
	num_requests BIGINT NOT NULL default 0
	)`
	_, err := s.db.Exec(query1)
	if err != nil {
		slog.Info("Got this error while trying to create table accounts", "error", err.Error())
		return err
	}
	query3 := `CREATE TYPE level AS ENUM ('easy', 'high', 'medium');`
	_, err3 := s.db.Exec(query3)
	if err3 != nil {
		slog.Info("Got this error while trying to create level enums", "error", err3.Error())
	}
	query2 := `CREATE TABLE IF NOT EXISTS Requests(
	id UUID primary key,
	cacheable bool,
	user_id varchar(50) REFERENCES Account(user_id),
	user_query TEXT NOT NULL, 
	llm_response TEXT NOT NULL, 
	input_tokens integer,
	output_tokens integer, 
	total_tokens integer,
	time_taken BIGINT,
	model varchar(50),
	cache_hit bool,
	level level
	)`
	_, err2 := s.db.Exec(query2)
	if err2 != nil {
		slog.Info("Got this error while trying to create table accounts", "error", err2.Error())
		return err2
	}
	slog.Info("Tables have been created!")
	return nil
}

// This function creates the userId if it doesn't exist in the db and then fetches it
//I guess this should be included in the increment tokens function only ...

func (s *PostgresStore) IncrementUserTokens(ctx context.Context, userId string, tokens int, level types.Level) error {
	var complex_tokens, simple_tokens int
	switch level {
	case types.Easy:
		simple_tokens = tokens
	case types.High:
		complex_tokens = tokens
	}

	ctx, cancelctx := context.WithTimeout(ctx, time.Second*3)
	defer cancelctx()
	const query = `
    INSERT INTO account (
		user_id,
		simple_tokens,
		complex_tokens,
		num_requests
		)
	VALUES (
		$1,
		$2,
		$3,
		1
		)
	ON CONFLICT (user_id) DO UPDATE
	SET
		simple_tokens  = account.simple_tokens  + EXCLUDED.simple_tokens,
		complex_tokens = account.complex_tokens + EXCLUDED.complex_tokens,
		num_requests   = account.num_requests   + 1
	RETURNING
		user_id,
		simple_tokens,
		complex_tokens,
		num_requests;
    `

	acc := &types.Account{}

	err := s.db.QueryRowContext(ctx, query, userId, simple_tokens, complex_tokens).Scan(
		&acc.UserId,
		&acc.Simple_Tokens,
		&acc.Complex_Tokens,
		&acc.Num_Requests,
	)
	if err != nil {
		return err
	}
	slog.Info("tokens incremented!", "account", acc)
	return nil
}

func (s *PostgresStore) InsertRequest(ctx context.Context, request types.Request) error {
	slog.Info("Adding a request into the db!")
	query := `INSERT INTO Requests(id, cacheable, user_id, user_query, llm_response, input_tokens, output_tokens, total_tokens, time_taken, model, cache_hit, level)
	VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	ctx, cancelctx := context.WithTimeout(ctx, time.Second*3)
	defer cancelctx()
	if _, err := s.db.ExecContext(ctx, query,
		request.Id,
		request.Cacheable,
		request.UserId,
		request.UserQuery,
		request.LLMResponse,
		request.InputTokens,
		request.OutputTokens,
		request.TotalToken,
		request.Time.Milliseconds(),
		request.Model,
		request.CacheHit,
		request.Level,
	); err != nil {
		slog.Info("Got an error while trying to insert this request into the postgres db", "error", err, "request", request)
		return err
	}
	return nil
}

func (s *PostgresStore) GetAnalytics() (types.AnalyticsResponse, error) {
	reqs, err := s.GetAllRequests()
	CostPerInputToken := 0.000002
	CostPerOutputToken := 0.000012
	if err != nil {
		slog.Error("Got this error while trying to calculate cost saved!", "error", err)
		return types.AnalyticsResponse{}, err
	}
	var CostSaved float64
	var CacheHitNum, CacheMissNum int64
	var TotalCacheMissTime, TotalCacheHitTime int64
	var TotalCacheMissTokens, TotalCacheHitTokens int

	slog.Info("Number of requests", "num", len(reqs))
	for _, r := range reqs {
		if r.CacheHit == true {
			CacheHitNum++
			TotalCacheHitTokens += r.TotalToken
			TotalCacheHitTime += r.Time.Milliseconds()
			// TimeSaved += time.Duration(AvgTimeTakenPerToken*r.TotalToken) - AvgTimeTakenByCachedRequest
			CostSaved += float64(r.InputTokens)*CostPerInputToken + float64(r.OutputTokens)*CostPerOutputToken
		} else {
			CacheMissNum++
			TotalCacheMissTime += r.Time.Milliseconds()
			TotalCacheMissTokens += r.TotalToken
		}
	}
	// avg cache miss time per token * total number of tokens - total time taken
	// AvgCacheHitTime := int64(TotalCacheHitTime.Milliseconds()/CacheHitNum)
	// AvgTimeTakenPerTokenOnCacheMiss := float64(TotalCacheMissTime) / float64(TotalCacheMissTokens)
	// slog.Info("the values here are ",
	// 	"AvgTimeTakenPerTokenOnCacheMiss", AvgTimeTakenPerTokenOnCacheMiss,
	// 	"TotalCacheHitTokens", TotalCacheHitTokens,
	// 	"TotalCacheMissTokens", TotalCacheMissTokens,
	// 	"TotalCacheHitTime", TotalCacheHitTime,
	// 	"TotalCacheMissTime", TotalCacheMissTime)
	// TimeSaved := float64(AvgTimeTakenPerTokenOnCacheMiss)*(float64(TotalCacheHitTokens)+float64(TotalCacheMissTokens)) - float64(TotalCacheHitTime+TotalCacheMissTime)
	//Time saved is total time taken if all requests were cache miss - total time taken in reality
	CacheHitPercentage := float64(CacheHitNum) / float64(len(reqs)) * 100
	slog.Info("Costs Saved", "num", CostSaved, "No. of Cache hits", CacheHitNum, "CacheHitPercentage", CacheHitPercentage)
	return types.AnalyticsResponse{
		CostSaved:          CostSaved,
		CacheHitPercentage: CacheHitPercentage,
		// TimeSaved:          time.Duration(TimeSaved * float64(time.Millisecond)),
		Msg: "Here are the analytics!",
	}, nil
}

// func (s *PostgresStore) GetAnalytics() (types.AnalyticsResponse, error) {
// 	reqs, err := s.GetAllRequests()
// 	CostPerInputToken := 0.000002
// 	CostPerOutputToken := 0.000012
// 	if err != nil {
// 		slog.Error("Got this error while trying to calculate cost saved!", "error", err)
// 		return types.AnalyticsResponse{}, err
// 	}
// 	var CostSaved float64
// 	var CacheHitNum, CacheMissNum int64
// 	var TotalCacheMissTime, TotalCacheHitTime int64
// 	var TotalCacheMissTokens, TotalCacheHitTokens int

// 	slog.Info("Number of requests", "num", len(reqs))
// 	for _, r := range reqs {
// 		if r.CacheHit == true {
// 			CacheHitNum++
// 			TotalCacheHitTokens += r.TotalToken
// 			TotalCacheHitTime += r.Time.Milliseconds()
// 			CostSaved += float64(r.InputTokens)*CostPerInputToken + float64(r.OutputTokens)*CostPerOutputToken
// 		} else {
// 			CacheMissNum++
// 			TotalCacheMissTime += r.Time.Milliseconds()
// 			TotalCacheMissTokens += r.TotalToken
// 		}
// 	}

// 	var TimeSaved int64
// 	if CacheMissNum > 0 && TotalCacheMissTokens > 0 && CacheHitNum > 0 {
// 		// Calculate average time per token for cache misses using float64
// 		AvgTimePerTokenOnCacheMiss := float64(TotalCacheMissTime) / float64(TotalCacheMissTokens)

// 		// Hypothetical time if cache hits were misses - Actual cache hit time
// 		TimeSavedMs := AvgTimePerTokenOnCacheMiss*float64(TotalCacheHitTokens) - float64(TotalCacheHitTime)
// 		TimeSaved = int64(TimeSavedMs)
// 	}

// 	CacheHitPercentage := float64(CacheHitNum) / float64(len(reqs)) * 100
// 	slog.Info("Costs Saved", "num", CostSaved, "No. of Cache hits", CacheHitNum, "CacheHitPercentage", CacheHitPercentage, "TimeSaved (ms)", TimeSaved)

// 	return types.AnalyticsResponse{
// 		CostSaved:          CostSaved,
// 		CacheHitPercentage: CacheHitPercentage,
// 		TimeSaved:          time.Duration(TimeSaved) * time.Millisecond,
// 		Msg:                "Here are the analytics!",
// 	}, nil
// }

func (s *PostgresStore) GetAllRequests() ([]*types.Request, error) {
	query := `SELECT 
	id, 
	cacheable, 
	user_id,
	user_query, 
	llm_response,
	input_tokens,
	output_tokens, 
	total_tokens,
	time_taken,
	model,
	cache_hit,
	level
	FROM Requests`
	row, err := s.db.Query(query)
	if err != nil {
		slog.Info("error occured while trying to get all requests", "error", err)
		return []*types.Request{}, err
	}
	var x []*types.Request
	defer row.Close()
	var t int64
	for row.Next() {
		var r = &types.Request{}
		err := row.Scan(
			&r.Id,
			&r.Cacheable,
			&r.UserId,
			&r.UserQuery,
			&r.LLMResponse,
			&r.InputTokens,
			&r.OutputTokens,
			&r.TotalToken,
			&t,
			&r.Model,
			&r.CacheHit,
			&r.Level,
		)
		slog.Info("t here is", "t", t)
		if err != nil {
			slog.Info("Got this error while trying to get all requests", "error", err)
			return x, err
		}
		r.Time = time.Duration(t * int64(time.Millisecond))
		x = append(x, r)
	}
	return x, nil
}
