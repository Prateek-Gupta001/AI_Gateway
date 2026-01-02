package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
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
	fmt.Println("got a request in generate response", w, messages, Level)
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
	fmt.Println("got a request in generate response", messages)
	client := &http.Client{}

	requestBody := map[string]interface{}{
		"model":  "gpt-4o", // Note: "gpt-5" does not exist yet; use "gpt-4o" or "o1-preview"
		"input":  CreateOpenAIMessages(messages),
		"stream": true,
	}

	// Marshaling handles all formatting, escaping, and whitespace correctly
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		slog.Error("Failed to marshal request", "error", err)
		return err
	}

	// Create the reader from the bytes
	var data = bytes.NewReader(jsonData)

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/responses", data)
	slog.Info("request made!", "req", req)
	if err != nil {
		slog.Error("error happened!", "error", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// req.Header.Set("Authorization", "Bearer "+" api key
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("error happened!", "error", err)
	}
	defer resp.Body.Close()
	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.Info("streaming unsupported!")
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
		slog.Info("LLMResStruct.LLMRes was nil")
		llmResStruct.LLMRes = new(bytes.Buffer)
	}
	f, err := os.OpenFile("streaming_output.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	// ... inside your loop
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println("err", err)
			break
		}

		// Write raw stream to file for your analysis
		f.WriteString(line)

		// Handle SSE parsing
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "data:") {
			jsonContent := strings.TrimPrefix(line, "data:")
			jsonContent = strings.TrimSpace(jsonContent)

			// 1. Check for specific event types
			// The JSON itself has a "type" field which is the source of truth
			var event ResponsesStreamEvent
			if err := json.Unmarshal([]byte(jsonContent), &event); err != nil {
				continue
			}

			switch event.Type {
			case "response.output_text.delta":
				slog.Info("event.delta", "info", event.Delta)
				fmt.Fprintf(w, "data: %s\n\n", line)
				flusher.Flush()

				if llmResStruct.LLMRes != nil {
					llmResStruct.LLMRes.WriteString(event.Delta)
				}

			case "response.completed":
				// Capture usage stats at the very end
				if event.Response != nil && event.Response.Usage != nil {
					fmt.Println("response.completed ", event.Response.Usage)
					llmResStruct.InputTokens = event.Response.Usage.InputTokens
					llmResStruct.OutputTokens = event.Response.Usage.OutputTokens
					llmResStruct.TotalTokens = event.Response.Usage.TotalTokens
				}
				// Break or continue as needed; the stream usually closes shortly after
			}
		}
	}
	llmResStruct.Level = types.Easy
	llmResStruct.Model = "GPT"
	fmt.Println("Returning from callGptAPI", llmResStruct)
	return nil
}

func callGeminiAPI(w http.ResponseWriter, messages []types.Messages, apikey string, llmResStruct *types.LLMResponse) error {
	return nil
}

func CreateOpenAIMessages(messages []types.Messages) []map[string]string {
	len := len(messages)
	msg := make([]map[string]string, 0, len)
	for _, m := range messages {
		msg = append(msg, map[string]string{
			"role": string(m.Role), "content": m.Content,
		})
	}
	fmt.Println("messages now looks like this", msg)
	return msg
}

type ErrorMessage struct {
	Message string `json:"message"`
	Type    string `json:"string"`
	Param   bool   `json:param`
	Code    string `json:code`
}
type ResponsesStreamEvent struct {
	Type           string `json:"type"`
	SequenceNumber int    `json:"sequence_number"`

	// Fields specific to different event types
	Delta        string    `json:"delta,omitempty"`    // For output_text.delta
	Response     *Response `json:"response,omitempty"` // For response.created/completed
	Item         *Item     `json:"item,omitempty"`     // For output_item.added/done
	OutputIndex  int       `json:"output_index,omitempty"`
	ContentIndex int       `json:"content_index,omitempty"`
	ItemId       string    `json:"item_id,omitempty"`
}

type Response struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "in_progress", "completed", "incomplete", "failed"
	Usage  *Usage `json:"usage,omitempty"`
}

type Item struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"` // e.g. "message"
	Role    string    `json:"role"`
	Content []Content `json:"content"`
	Status  string    `json:"status"`
}

type Content struct {
	Type string `json:"type"` // "output_text"
	Text string `json:"text"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
