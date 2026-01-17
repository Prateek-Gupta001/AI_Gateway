package llm

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/Prateek-Gupta001/AI_Gateway/types"
)

func TestCallGptAPI_Integration(t *testing.T) {
	// 1. Safety Check: Ensure API key is present
	fmt.Println("PRINTING!!!")
	apiKey := ""
	if apiKey == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	// 2. Setup the ResponseRecorder (This acts as your http.ResponseWriter)
	// httptest.NewRecorder automatically implements http.Flusher, so your
	// casting check `w.(http.Flusher)` will pass.
	recorder := httptest.NewRecorder()

	// 3. Prepare your data container
	llmResStruct := &types.LLMResponse{
		LLMRes: new(bytes.Buffer),
	}

	// 4. Call the function
	// Note: Your current implementation hardcodes the input prompt inside callGptAPI,
	// so the 'messages' argument here is ignored, but we pass nil for now.
	err := CallGptAPI(context.Background(), recorder, nil, apiKey, llmResStruct)

	// 5. Assertions
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// CHECK 1: Did we capture text in the struct?
	gotText := llmResStruct.LLMRes.String()
	t.Logf("Full LLM Response: %s", gotText)

	if gotText == "" {
		t.Error("Expected LLM response text, got empty string")
	}

	// CHECK 2: Did the token usage get parsed?
	if llmResStruct.TotalTokens == 0 {
		t.Error("Expected TotalTokens to be > 0, failed to parse usage stats")
	}
	t.Logf("Token Usage - Input: %d, Output: %d, Total: %d",
		llmResStruct.InputTokens, llmResStruct.OutputTokens, llmResStruct.TotalTokens)

	// CHECK 3: Did it stream to the HTTP writer?
	// The recorder.Body will contain the raw data written to w (e.g. "Aye, matey!")
	// Note: Since your code writes just the delta (text) to 'w', this should match LLMRes.
	streamedOutput := recorder.Body.String()
	if streamedOutput != gotText {
		t.Errorf("Mismatch between Struct storage and HTTP stream.\nStruct: %s\nStream: %s", gotText, streamedOutput)
	}
}
