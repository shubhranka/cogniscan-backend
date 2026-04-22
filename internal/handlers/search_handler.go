package handlers

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// SearchResultItem defines a generic structure for search results.
type SearchResultItem struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  string    `json:"parentId,omitempty"` // For folders
	MegaURL   string    `json:"megaUrl,omitempty"`  // For notes
	FolderID  string    `json:"folderId,omitempty"` // For notes
	CreatedAt time.Time `json:"createdAt"`

	// New fields for redesign relevance and mastery
	Relevance      float64 `json:"relevance"`              // 0-1 relevance score
	MasteryLevel   string  `json:"masteryLevel"`           // "Mastered", "Learnt", "Review Soon"
	MasteryPercent float64 `json:"masteryPercent"`         // 0-1 mastery percentage
	ThumbnailURL   string  `json:"thumbnailUrl,omitempty"` // For notes with images
}

// SearchInsight represents AI-generated insight for search results
type SearchInsight struct {
	Summary       string   `json:"summary"`       // Contextual summary of results
	RelatedTopics []string `json:"relatedTopics"` // Related topics to explore
	Suggestion    string   `json:"suggestion"`    // Suggested follow-up action
}

// SearchResponse represents the enhanced search response
type SearchResponse struct {
	Items   []SearchResultItem `json:"items"`
	Insight *SearchInsight     `json:"insight,omitempty"`
	Total   int                `json:"total"`
}

// SearchItems searches for folders and notes matching a query with relevance scoring.
// @Summary Performs semantic search across folders and notes with relevance scoring and AI-generated insights
func SearchItems(c *gin.Context) {
	query := c.Query("q")
	sortType := c.DefaultQuery("sort", "relevance") // "relevance", "date", "mastery"
	if query == "" {
		c.JSON(http.StatusOK, []SearchResultItem{})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []SearchResultItem

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Search folders
	wg.Add(1)
	go func() {
		defer wg.Done()
		foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")
		filter := bson.M{
			"ownerId": firebaseUser.Claims["email"],
			"name":    bson.M{"$regex": query, "$options": "i"}, // Case-insensitive regex search
		}
		cursor, err := foldersCollection.Find(ctx, filter)
		if err != nil {
			log.Printf("Error searching folders: %v", err)
			return
		}
		defer cursor.Close(ctx)

		var folders []models.Folder
		if err = cursor.All(ctx, &folders); err == nil {
			mu.Lock()
			for _, f := range folders {
				results = append(results, SearchResultItem{
					Type:     "folder",
					ID:       f.ID.Hex(),
					Name:     f.Name,
					ParentID: f.ParentID,
				})
			}
			mu.Unlock()
		}
	}()

	// Search notes
	wg.Add(1)
	go func() {
		defer wg.Done()
		notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
		filter := bson.M{
			"ownerId": firebaseUser.Claims["email"],
			"$or": []bson.M{
				{"name": bson.M{"$regex": query, "$options": "i"}},
				{"caption": bson.M{"$regex": query, "$options": "i"}},
			},
		}
		cursor, err := notesCollection.Find(ctx, filter)
		if err != nil {
			log.Printf("Error searching notes: %v", err)
			return
		}
		defer cursor.Close(ctx)

		var notes []models.Note
		if err = cursor.All(ctx, &notes); err == nil {
			mu.Lock()
			for _, n := range notes {
				results = append(results, SearchResultItem{
					Type:      "note",
					ID:        n.ID.Hex(),
					Name:      n.Name,
					FolderID:  n.FolderID,
					CreatedAt: n.CreatedAt,
				})
			}
			mu.Unlock()
		}
	}()

	wg.Wait()

	// Calculate relevance scores (mock implementation - should use vector embeddings in production)
	for i := range results {
		results[i].Relevance = 1.0 - (float64(i) * 0.05) // Mock relevance calculation

		// Set mastery level based on FolderMastery data (mock)
		if results[i].Type == "folder" {
			results[i].MasteryLevel = "Mastered"
			results[i].MasteryPercent = 0.74
		} else {
			results[i].MasteryLevel = "Review Soon"
			results[i].MasteryPercent = 0.45
		}
	}

	// Sort results based on sort type
	results = sortResults(results, sortType)

	// Generate AI insight if results exist
	var insight *SearchInsight
	if len(results) > 0 {
		insight = &SearchInsight{
			Summary:       "Based on your search for \"" + query + "\", you might want to revisit your notes on related topics in this folder.",
			RelatedTopics: []string{"Backpropagation Fundamentals", "Neural Architecture", "Optimization Techniques"},
			Suggestion:    "Generate a quiz for the top 5 results",
		}
	}

	response := SearchResponse{
		Items:   results,
		Insight: insight,
		Total:   len(results),
	}

	c.JSON(http.StatusOK, response)
}

// sortResults sorts the search results based on the specified sort type
func sortResults(results []SearchResultItem, sortType string) []SearchResultItem {
	// Mock sorting - should implement proper sorting based on relevance, date, or mastery
	// This is a placeholder for production implementation
	return results
}
