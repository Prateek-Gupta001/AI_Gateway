package types

import (
	"bytes"
	"time"
)

type Embedding [][]float32

type Level string

const (
	Easy   Level = "easy"
	High   Level = "high"
	Medium Level = "medium"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

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
	Err              error
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
	InputTokens  int
	OutputTokens int
	TotalToken   int
	Time         time.Duration
	Model        string
	CacheHit     bool
	Level        Level
}
