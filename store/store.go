package store

import (
	"database/sql"
	"log/slog"
	"time"

	_ "github.com/lib/pq"

	"github.com/Prateek-Gupta001/AI_Gateway/types"
)

type Storage interface {
	InsertRequest(types.Request) error
	GetCacheHitPercentage() (int, error)
	GetTimeSaved() (time.Duration, error)
	CostSaved() (int, error)
	IncrementUserTokens(userId string, tokens int, level types.Level) error
	GetAllRequests() ([]*types.Request, error)
}

type PostgresStore struct {
	db *sql.DB
}

func NewStorage() (*PostgresStore, error) {
	connStr := "host=127.0.0.1 port=5432 user=postgres dbname=postgres password=gobank sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		slog.Info("Got this error while trying to open a connection to the database ", "error", err)
		return nil, err
	}
	if err = db.Ping(); err != nil {
		slog.Info("Got this error while trying to ping the database ", "error", err)
		return nil, err
	}
	return &PostgresStore{
		db: db,
	}, nil
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

func (s *PostgresStore) IncrementUserTokens(userId string, tokens int, level types.Level) error {
	var complex_tokens, simple_tokens int
	switch level {
	case types.Easy:
		simple_tokens = tokens
	case types.High:
		complex_tokens = tokens
	}

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

	err := s.db.QueryRow(query, userId, simple_tokens, complex_tokens).Scan(
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

func (s *PostgresStore) InsertRequest(request types.Request) error {
	slog.Info("Adding a request into the db!")
	query := `INSERT INTO Requests(id, cacheable, user_id, user_query, llm_response, input_tokens, output_tokens, total_tokens, time_taken, model, cache_hit, level)
	VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	if _, err := s.db.Exec(query,
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

func (s *PostgresStore) GetCacheHitPercentage() (int, error) {
	return 0, nil
}

func (s *PostgresStore) GetTimeSaved() (time.Duration, error) {
	return time.Duration(time.Second), nil
}

func (s *PostgresStore) CostSaved() (int, error) {
	return 0, nil
}

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
	var i int
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
		if err != nil {
			slog.Info("Got this error while trying to get all requests", "error", err)
			return x, err
		}
		r.Time = time.Duration(t)
		i = i + 1
		x = append(x, r)
	}
	return x, nil

}
