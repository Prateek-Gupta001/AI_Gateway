package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Prateek-Gupta001/AI_Gateway/types"
)

type LLMs interface {
	GenerateResponse(http.ResponseWriter, []types.Messages, types.Level, *types.LLMResponse) error
}

type llmModel struct {
	Level     types.Level
	ModelName string
	ApiKey    string
	Call      LLMProvider
}

type LLMProvider func(w http.ResponseWriter, messages []types.Messages, apikey string, llmResStruct *types.LLMResponse) error

type LLMStruct struct {
	Models []llmModel
}

func (s *LLMStruct) GenerateResponse(w http.ResponseWriter, messages []types.Messages, Level types.Level, llmResStruct *types.LLMResponse) error {
	//could employ a strategy here to ensure that the ones giving off the error a lot of the time is not selected!
	//also .. make a fake .. http buffer/stream .. that I could then use .. to test things .. and actually show this running!
	for _, llm := range s.Models {
		if llm.Level == Level {
			return llm.Call(w, messages, llm.ApiKey, llmResStruct)
		}
	}
	return fmt.Errorf("Invalid Level type/ Not present in LLMStruct")
}

func NewLLMStruct() *LLMStruct {
	return &LLMStruct{
		Models: []llmModel{{ModelName: "Gpt 4o", ApiKey: "dummyapikey", Level: types.Easy, Call: callGptAPI},
			{ModelName: "Gemini 3.0", ApiKey: "dummyapikey2", Level: types.High, Call: callGeminiAPI}},
	}
}

func callGptAPI(w http.ResponseWriter, messages []types.Messages, apikey string, llmResStruct *types.LLMResponse) error {
	resp, err := http.Get("http://localhost:8080/test-stream")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return nil
	}
	if err != nil {
		fmt.Println("Got this err ", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if llmResStruct.LLMRes == nil {
		llmResStruct.LLMRes = new(bytes.Buffer)
	}
	for {
		data, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break // stream ended
			}
			// real error
			fmt.Println("err ", err)
		}
		//use io.multiwriter here .. and write to the both the things ... the llmResStruct and to the http.responsewriter!
		fmt.Fprint(w, data)
		flusher.Flush()
		var chunk = &OpenAIChunk{}
		if strings.HasPrefix(data, "data:") {
			dataContent := strings.TrimPrefix(data, "data:")
			dataContent = strings.TrimSpace(dataContent)
			if dataContent == "[DONE]" {
				continue
			}
			if err := json.Unmarshal([]byte(dataContent), chunk); err != nil {
				slog.Info("Got this error while trying to unmarshal the given chunk to json!", "error", err.Error(), "chunk", dataContent)
				continue
			}
			if chunk.Usage != nil {
				llmResStruct.InputTokens = chunk.Usage.PromptTokens
				llmResStruct.OutputTokens = chunk.Usage.CompletionTokens
				llmResStruct.TotalTokens = chunk.Usage.TotalTokens
			}

			if len(chunk.Choices) != 0 {
				content := chunk.Choices[0].Delta.Content
				if content != "" {
					llmResStruct.LLMRes.WriteString(chunk.Choices[0].Delta.Content)
				}
			}
		}
	}
	llmResStruct.Level = types.Easy
	llmResStruct.Model = "GPT"
	return nil
}

func callGeminiAPI(w http.ResponseWriter, messages []types.Messages, apikey string, llmResStruct *types.LLMResponse) error {
	return nil
}

type OpenAIDelta struct {
	Content string `json:"content,omitempty"`
}

type OpenAIChoice struct {
	Index        int         `json:"index"`
	Delta        OpenAIDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   *OpenAIUsage   `json:"usage,omitempty"` // Only present in the final chunk
}
