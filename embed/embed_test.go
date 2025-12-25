package embed

// import (
// 	"fmt"
// 	"log/slog"
// 	"testing"
// )

// func TestCreateEmbeddings(t *testing.T) {
// 	e := NewEmbeddingModel()
// 	slog.Info("new embedding model ", "value", e)
// 	defer e.Close()
// 	userQuery := "Hey there who are you?"
// 	embedding, err := e.CreateQueryEmbedding(userQuery)
// 	if err != nil {
// 		slog.Info("got this error while trying to create embeddings", "error", err.Error())
// 	}
// 	somethingElse := len(embedding[0])
// 	fmt.Println("The actual dim is ", " ", somethingElse)
// }
