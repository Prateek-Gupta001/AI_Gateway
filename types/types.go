package types

import (
	"bytes"
	"context"
	"time"
)

type Embedding []float32

type Level string

const (
	Easy   Level = "easy"
	High   Level = "high"
	Medium Level = "medium"
)

var AllLevels = []Level{
	Easy,
	Medium,
	High,
}

type ctxKey int

const UserIdKey ctxKey = iota

type IncTokenPayload struct {
	UserId string
	Tokens int
	Level  Level
	Ctx    context.Context
}

type InsertRequestPayload struct {
	Request Request
	Ctx     context.Context
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

type AnalyticsResponse struct {
	CostSaved          float64
	CacheHitPercentage float64
	Msg                string
}

type LLMResponse struct {
	LLMRes       *bytes.Buffer
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Model        string
	Level        Level
}

type EmbeddingResult struct {
	Embedding_Result Embedding
	Query            string
	Err              error
}

type EmbeddingJob struct {
	Ctx        context.Context
	Input      string
	ResultChan chan EmbeddingResult
}

type Messages struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type RequestStruct struct {
	UserId    string     `json:"userId"`
	Messages  []Messages `json:"messages"`
	CacheFlag bool
}

type CacheResponse struct {
	InputTokens  int
	OutputTokens int
	CachedAnswer string
	CachedQuery  string
}

type Account struct {
	UserId         string
	Simple_Tokens  int
	Complex_Tokens int
	Num_Requests   int
}

type Request struct {
	Id           string
	Cacheable    bool
	UserId       string
	UserQuery    string
	LLMResponse  string
	InputTokens  int
	OutputTokens int
	TotalToken   int
	Time         time.Duration
	Model        string
	CacheHit     bool
	Level        Level
}
