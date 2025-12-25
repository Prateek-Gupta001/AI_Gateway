package cache

import (
	"github.com/Prateek-Gupta001/AI_Gateway/types"
	"github.com/qdrant/go-client/qdrant"
)

type Cache interface {
	ExistsInCache(Embedding types.Embedding, userQuery string) (types.CacheResponse, bool, error) //if found then "query answer", true, nil ..If not found then "", false, nil ..
	InsertIntoCache(Embedding types.Embedding, LLMAnswer string, userQuery string) error          //LLMAnswer will be stored in qdrant metadata!
}

type QdrantCache struct {
	Client    qdrant.Client
	Threshold float32
}

func NewQdrantCache() *QdrantCache {
	return &QdrantCache{
		Client:    qdrant.Client{},
		Threshold: 0.95,
	}
}

func (q *QdrantCache) ExistsInCache(Embedding types.Embedding, userQuery string) (types.CacheResponse, bool, error) {

	return types.CacheResponse{}, true, nil
}

func (q *QdrantCache) InsertIntoCache(Embedding types.Embedding, LLMAnswer string, userQuery string) error {
	return nil
}
