// ./cogniscan-backend/internal/handlers/search_handler.go
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
}

// SearchItems searches for folders and notes matching a query.
func SearchItems(c *gin.Context) {
	query := c.Query("q")
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
			"ownerId": firebaseUser.UID,
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
			"ownerId": firebaseUser.UID,
			"name":    bson.M{"$regex": query, "$options": "i"},
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
					MegaURL:   n.MegaURL,
					FolderID:  n.FolderID,
					CreatedAt: n.CreatedAt,
				})
			}
			mu.Unlock()
		}
	}()

	wg.Wait()

	c.JSON(http.StatusOK, results)
}
