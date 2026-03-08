package services

import (
	"context"
	"log"
	"os"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	vectorCollectionName = "caption_embeddings"
	vectorIndexName      = "vector_index"
	vectorDimension      = 1024 // llama-nemotron-embed-1b-v2 produces 1024 dimensions
)

// getVectorCollection returns the collection for caption embeddings
func getVectorCollection() *mongo.Collection {
	return database.Client.Database(os.Getenv("DB_NAME")).Collection(vectorCollectionName)
}

// EnsureVectorIndex ensures the vector index exists for semantic search
// Note: For MongoDB Atlas Vector Search, you need to create the index via the Atlas UI or API
// The index definition should be:
// {
//   "fields": [
//     {
//       "numDimensions": 1024,
//       "path": "vector",
//       "similarity": "cosine",
//       "type": "vector"
//     },
//     {
//       "path": "ownerId",
//       "type": "filter"
//     },
//     {
//       "path": "folderId",
//       "type": "filter"
//     }
//   ]
// }
func EnsureVectorIndex() error {
	collection := getVectorCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check if index already exists
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return err
	}

	hasVectorIndex := false
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err == nil {
			if name, ok := index["name"].(string); ok && name == vectorIndexName {
				hasVectorIndex = true
				break
			}
		}
	}

	if hasVectorIndex {
		log.Printf("[VectorService] Vector index '%s' already exists", vectorIndexName)
		return nil
	}

	// For MongoDB Atlas Vector Search, the index needs to be created via Atlas
	// We'll create a regular compound index as a fallback for filtering
	indexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "ownerId", Value: 1},
			{Key: "folderId", Value: 1},
		},
		Options: options.Index().SetName("owner_folder_index"),
	}

	_, err = collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		log.Printf("[VectorService] Warning: Could not create backup index: %v", err)
	}

	log.Println("[VectorService] Vector search index not found. Please create it via MongoDB Atlas UI:")
	log.Println("  Navigate to: Collections > caption_embeddings > Create Search Index")
	log.Println("  Use the following JSON definition:")
	log.Println(`  {
    "fields": [
      {
        "numDimensions": 1024,
        "path": "vector",
        "similarity": "cosine",
        "type": "vector"
      },
      {
        "path": "ownerId",
        "type": "filter"
      },
      {
        "path": "folderId",
        "type": "filter"
      }
    ]
  }`)

	return nil
}

// StoreCaptionEmbedding stores a caption with its embedding vector
func StoreCaptionEmbedding(noteID, folderID, ownerID, caption string, vector []float32) error {
	collection := getVectorCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use upsert to update existing embedding or insert new one
	filter := bson.M{"noteId": noteID}
	update := bson.M{
		"$set": bson.M{
			"noteId":    noteID,
			"folderId":  folderID,
			"ownerId":   ownerID,
			"caption":   caption,
			"vector":    vector,
			"updatedAt": time.Now(),
		},
		"$setOnInsert": bson.M{
			"createdAt": time.Now(),
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		log.Printf("[VectorService] Failed to store embedding for note %s: %v", noteID, err)
		return err
	}

	log.Printf("[VectorService] Stored embedding for note %s", noteID)
	return nil
}

// SearchSimilarCaptions performs vector similarity search to find similar captions
// Returns captions with their similarity scores (0-1, higher is more similar)
func SearchSimilarCaptions(query string, limit int, ownerID string) ([]models.CaptionEmbedding, []float32, error) {
	// Generate query embedding for the search text (uses "query" input_type)
	queryVector, err := GenerateQueryEmbedding(query)
	if err != nil {
		return nil, nil, err
	}

	collection := getVectorCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build aggregation pipeline for vector search
	pipeline := mongo.Pipeline{
		// Vector search stage
		bson.D{
			{Key: "$vectorSearch", Value: bson.M{
				"index":            vectorIndexName,
				"path":             "vector",
				"queryVector":      queryVector,
				"numCandidates":    limit * 10, // Get more candidates for better recall
				"limit":            limit,
				"filter":           bson.M{"ownerId": ownerID}, // Only search within user's data
			}},
		},
		// Project stage to include the score
		bson.D{
			{Key: "$project", Value: bson.M{
				"_id":       1,
				"noteId":    1,
				"folderId":  1,
				"ownerId":   1,
				"caption":   1,
				"createdAt": 1,
				"updatedAt": 1,
				"score": bson.M{
					"$meta": "vectorSearchScore",
				},
			}},
		},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var results []models.CaptionEmbedding
	var scores []float32

	for cursor.Next(ctx) {
		var result struct {
			models.CaptionEmbedding
			Score float32 `bson:"score"`
		}

		if err := cursor.Decode(&result); err != nil {
			continue
		}

		results = append(results, result.CaptionEmbedding)
		scores = append(scores, result.Score)
	}

	return results, scores, nil
}

// DeleteCaptionEmbedding removes the embedding for a specific note
func DeleteCaptionEmbedding(noteID string) error {
	collection := getVectorCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"noteId": noteID}
	result, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Printf("[VectorService] Failed to delete embedding for note %s: %v", noteID, err)
		return err
	}

	if result.DeletedCount > 0 {
		log.Printf("[VectorService] Deleted embedding for note %s", noteID)
	}

	return nil
}

// DeleteFolderEmbeddings removes all embeddings for notes in a folder
func DeleteFolderEmbeddings(folderID string) error {
	collection := getVectorCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"folderId": folderID}
	result, err := collection.DeleteMany(ctx, filter)
	if err != nil {
		log.Printf("[VectorService] Failed to delete embeddings for folder %s: %v", folderID, err)
		return err
	}

	if result.DeletedCount > 0 {
		log.Printf("[VectorService] Deleted %d embeddings for folder %s", result.DeletedCount, folderID)
	}

	return nil
}

// GetCaptionEmbedding retrieves the embedding for a specific note
func GetCaptionEmbedding(noteID string) (*models.CaptionEmbedding, error) {
	collection := getVectorCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"noteId": noteID}
	var embedding models.CaptionEmbedding
	err := collection.FindOne(ctx, filter).Decode(&embedding)
	if err != nil {
		return nil, err
	}

	return &embedding, nil
}

// SearchCaptionsInFolder performs vector search within a specific folder
func SearchCaptionsInFolder(query string, limit int, folderID, ownerID string) ([]models.CaptionEmbedding, []float32, error) {
	// Generate query embedding for the search text (uses "query" input_type)
	queryVector, err := GenerateQueryEmbedding(query)
	if err != nil {
		return nil, nil, err
	}

	collection := getVectorCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pipeline := mongo.Pipeline{
		bson.D{
			{Key: "$vectorSearch", Value: bson.M{
				"index":       vectorIndexName,
				"path":        "vector",
				"queryVector": queryVector,
				"numCandidates": limit * 10,
				"limit":       limit,
				"filter": bson.M{
					"folderId": folderID,
					"ownerId":  ownerID,
				},
			}},
		},
		bson.D{
			{Key: "$project", Value: bson.M{
				"_id":       1,
				"noteId":    1,
				"folderId":  1,
				"ownerId":   1,
				"caption":   1,
				"createdAt": 1,
				"updatedAt": 1,
				"score": bson.M{
					"$meta": "vectorSearchScore",
				},
			}},
		},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var results []models.CaptionEmbedding
	var scores []float32

	for cursor.Next(ctx) {
		var result struct {
			models.CaptionEmbedding
			Score float32 `bson:"score"`
		}

		if err := cursor.Decode(&result); err != nil {
			continue
		}

		results = append(results, result.CaptionEmbedding)
		scores = append(scores, result.Score)
	}

	return results, scores, nil
}

// InitVectorService initializes the vector service by ensuring the vector index exists
func InitVectorService() error {
	return EnsureVectorIndex()
}
