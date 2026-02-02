package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"strings"

	"github.com/Prateek-Gupta001/AI_Gateway/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type LLMs interface {
	GenerateResponse(context.Context, http.ResponseWriter, []types.Messages, types.Level, *types.LLMResponse) error
}

var Tracer = otel.Tracer("ai-gateway-service")

type llmModel struct {
	Level     types.Level
	ModelName string
	ApiKey    string
	Call      LLMProvider
}

type LLMProvider func(ctx context.Context, w http.ResponseWriter, messages []types.Messages, apikey string, llmResStruct *types.LLMResponse) error

type LLMStruct struct {
	Models []llmModel
}

func (s *LLMStruct) ChangeModel(modelname string) llmModel {
	//In future for making sure that model level changes if at that level the model failed!
	for _, model := range s.Models {
		if modelname != model.ModelName {
			slog.Info("Returning a changed model", "earlier level", modelname, "new level", model.ModelName)
			return model
		}
	}
	slog.Error("Critical Error! Model change couldn't take place ... models weren't intialised properly")
	return s.Models[0]
}

func (s *LLMStruct) GenerateResponse(ctx context.Context, w http.ResponseWriter, messages []types.Messages, Level types.Level, llmResStruct *types.LLMResponse) error {
	fmt.Println("got a request in generate response", w, messages, Level)
	//could employ a strategy here to ensure that the ones giving off the error a lot of the time is not selected!
	//also .. make a fake .. http buffer/stream .. that I could then use .. to test things .. and actually show this running!
	for _, llm := range s.Models {
		if llm.Level == Level {
			return llm.Call(ctx, w, messages, llm.ApiKey, llmResStruct)
		}
	}
	return fmt.Errorf("Invalid Level type/ Not present in LLMStruct")
}

func NewLLMStruct() *LLMStruct {
	return &LLMStruct{
		Models: []llmModel{{ModelName: "Gpt 4o", ApiKey: os.Getenv("OPENAI_API_KEY"), Level: types.Easy, Call: MockCallGptAPI},
			{ModelName: "Gemini 2.5 flash", ApiKey: os.Getenv("GEMINI_API_KEY"), Level: types.High, Call: CallGeminiAPI}},
	}
}

func CallGptAPI(ctx context.Context, w http.ResponseWriter, messages []types.Messages, apikey string, llmResStruct *types.LLMResponse) error {
	fmt.Println("got a request in generate response", messages)
	ctx, span := Tracer.Start(ctx, "CallGptAPI")
	defer span.End()
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
	req.Header.Set("Authorization", "Bearer "+apikey)
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
	reader := bufio.NewReader(resp.Body)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if llmResStruct.LLMRes == nil {
		slog.Info("LLMResStruct.LLMRes was nil")
		llmResStruct.LLMRes = new(bytes.Buffer)
	}

	// ... inside your loop
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println("err", err)
			return err //TODO: DO something here to handle the errors even inside the streams gracefully!
		}

		// Handle SSE parsing
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "data:") {
			jsonContent := strings.TrimPrefix(line, "data:")
			jsonContent = strings.TrimSpace(jsonContent)

			// 1. Check for specific event types
			// The JSON itself has a "type" field which is the source of truth
			var event ResponsesStreamEvent
			if err := json.Unmarshal([]byte(jsonContent), &event); err != nil {
				slog.Error("Got this error while trying to unmarshal the json in OpenAI", "error", err)
				continue
			}

			switch event.Type {
			case "response.output_text.delta":
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

func MockCallGptAPI(ctx context.Context, w http.ResponseWriter, messages []types.Messages, apikey string, llmResStruct *types.LLMResponse) error {
	ctx, span := Tracer.Start(ctx, "MockCallGptAPI")
	defer span.End()

	resp, err := http.Get("http://localhost:8082/test-stream")
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
	span.SetAttributes(
		attribute.Int("total_tokens", llmResStruct.TotalTokens),
		attribute.Int("input_tokens", llmResStruct.InputTokens),
		attribute.Int("output_tokens", llmResStruct.OutputTokens),
		attribute.String("model", llmResStruct.Model),
		attribute.String("llmResponse", llmResStruct.LLMRes.String()),
		attribute.String("level", string(llmResStruct.Level)),
	)
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

func CreateGeminiMessages(messages []types.Messages) []map[string]interface{} {
	len := len(messages)
	msg := make([]map[string]interface{}, 0, len)
	for _, m := range messages {
		if m.Role == types.RoleAssistant {
			m.Role = "model"
		}
		msg = append(msg, map[string]interface{}{
			"role": string(m.Role),
			"parts": []map[string]string{
				{"text": m.Content},
			},
		})
	}
	return msg
}

func CallGeminiAPI(ctx context.Context, w http.ResponseWriter, messages []types.Messages, apikey string, llmResStruct *types.LLMResponse) error {
	ctx, span := Tracer.Start(ctx, "CallGeminiAPI")
	defer span.End()
	client := &http.Client{}
	jsonRequest := map[string]interface{}{
		"contents": CreateGeminiMessages(messages),
	}
	jsonData, err := json.Marshal(jsonRequest)
	if err != nil {
		slog.Error("Got this error while trying to marshal the llm request into json", "error", err)
	}
	finalReq := bytes.NewReader(jsonData)
	req, err := http.NewRequest("POST", "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse", finalReq)
	//use a non thinking model only!
	if err != nil {
		slog.Error("Got this error right here", "error", err)
		return err
	}
	req.Header.Set("x-goog-api-key", apikey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Got this error right here", "error", err)
	}
	defer resp.Body.Close()
	flusher := w.(http.Flusher)
	reader := bufio.NewReader(resp.Body)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if llmResStruct.LLMRes == nil {
		slog.Info("LLMResStruct.LLMRes was nil")
		llmResStruct.LLMRes = new(bytes.Buffer)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			slog.Error("Got this unexpected error inside the string", "error", err)
			return err
		}
		fmt.Fprintf(w, line)
		flusher.Flush()
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			jsonContent := strings.TrimPrefix(line, "data:")
			jsonContent = strings.TrimSpace(jsonContent)

			var chunk GeminiStreamResponse
			err := json.Unmarshal([]byte(jsonContent), &chunk)
			if err != nil {
				slog.Error("Got this error while trying to unmarshal the json in Gemini", "error", err)
			}
			if len(chunk.Candidates) > 0 && len(chunk.Candidates[0].Content.Parts) > 0 {
				textChunk := chunk.Candidates[0].Content.Parts[0].Text
				llmResStruct.LLMRes.WriteString(textChunk)
			}

			if chunk.UsageMetadata != nil {
				llmResStruct.InputTokens = chunk.UsageMetadata.PromptTokenCount
				llmResStruct.OutputTokens = chunk.UsageMetadata.CandidatesTokenCount
				llmResStruct.TotalTokens = chunk.UsageMetadata.TotalTokenCount
			}

		}

	}
	llmResStruct.Model = "Gemini"
	llmResStruct.Level = types.High

	return nil
}

// GeminiStreamResponse represents the root JSON object received in each SSE chunk.
type GeminiStreamResponse struct {
	Candidates    []Candidate    `json:"candidates"`
	UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"` // Pointer as it's not always present
}

type Candidate struct {
	Content      Gemini_Content `json:"content"`
	FinishReason string         `json:"finishReason"`
	Index        int            `json:"index"`
}

type Gemini_Content struct {
	Parts []Part `json:"parts"`
	Role  string `json:"role"`
}

type Part struct {
	Text string `json:"text"`
}

// UsageMetadata captures the token counts.
// This is usually sent in the final chunk of the stream.
type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
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

type ResponsesStreamEvent struct {
	Type           string `json:"type"`
	SequenceNumber int    `json:"sequence_number"`

	// Fields specific to different event types
	Delta        string    `json:"delta,omitempty"`
	Response     *Response `json:"response,omitempty"`
	Item         *Item     `json:"item,omitempty"`
	OutputIndex  int       `json:"output_index,omitempty"`
	ContentIndex int       `json:"content_index,omitempty"`
	ItemId       string    `json:"item_id,omitempty"`
}

type Response struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Usage  *Usage `json:"usage,omitempty"`
}

type Item struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Role    string    `json:"role"`
	Content []Content `json:"content"`
	Status  string    `json:"status"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
