// ./cogniscan-backend/internal/handlers/folder_handler.go
package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CreateFolderPayload defines the expected JSON for creating a folder
type CreateFolderPayload struct {
	Name     string `json:"name" binding:"required"`
	ParentID string `json:"parentId"` // Can be empty for root folders
}

// CreateFolder is correct and unchanged.
func CreateFolder(c *gin.Context) {
	var payload CreateFolderPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	newFolder := models.Folder{
		ID:       primitive.NewObjectID(),
		Name:     payload.Name,
		ParentID: payload.ParentID,
		OwnerID:  firebaseUser.UID,
	}
	foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := foldersCollection.InsertOne(ctx, newFolder)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create folder"})
		return
	}
	c.JSON(http.StatusCreated, newFolder)
}

// GetFolders is correct and unchanged.
func GetFolders(c *gin.Context) {
	parentID := c.Param("folderId")
	if parentID == "root" {
		parentID = ""
	}
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{
		"ownerId":  firebaseUser.UID,
		"parentId": parentID,
	}
	cursor, err := foldersCollection.Find(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch folders"})
		return
	}
	defer cursor.Close(ctx)
	var folders []models.Folder
	if err = cursor.All(ctx, &folders); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode folders"})
		return
	}
	if folders == nil {
		folders = make([]models.Folder, 0)
	}
	c.JSON(http.StatusOK, folders)
}

// UpdateFolderPayload defines the expected JSON for renaming a folder
type UpdateFolderPayload struct {
	Name string `json:"name" binding:"required"`
}

// UpdateFolder renames a folder.
func UpdateFolder(c *gin.Context) {
	folderID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid folder ID"})
		return
	}
	var payload UpdateFolderPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{"_id": folderID, "ownerId": firebaseUser.UID}
	update := bson.M{"$set": bson.M{"name": payload.Name}}
	result, err := foldersCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update folder"})
		return
	}
	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Folder not found or you don't have permission"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Folder updated successfully"})
}

// DeleteFolder deletes a folder and all its contents recursively from the database.
func DeleteFolder(c *gin.Context) {
	folderIDHex := c.Param("id")
	folderID, err := primitive.ObjectIDFromHex(folderIDHex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid folder ID"})
		return
	}
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err = deleteFolderRecursively(ctx, folderIDHex, firebaseUser.UID)
	if err != nil {
		log.Printf("Error during recursive delete: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete folder and its contents"})
		return
	}
	foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")
	_, err = foldersCollection.DeleteOne(ctx, bson.M{"_id": folderID, "ownerId": firebaseUser.UID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete the main folder"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Folder deleted successfully"})
}

func deleteFolderRecursively(ctx context.Context, parentIDHex, ownerID string) error {
	dbName := os.Getenv("DB_NAME")
	foldersCollection := database.Client.Database(dbName).Collection("folders")
	notesCollection := database.Client.Database(dbName).Collection("notes")

	log.Printf("Deleting notes in folder %s from database. MEGA files will be orphaned.", parentIDHex)

	// Delete all notes records from MongoDB for this folder
	_, err := notesCollection.DeleteMany(ctx, bson.M{"folderId": parentIDHex, "ownerId": ownerID})
	if err != nil {
		return fmt.Errorf("failed to delete notes from db for folder %s: %v", parentIDHex, err)
	}

	// Find and recurse through subfolders
	folderCursor, err := foldersCollection.Find(ctx, bson.M{"parentId": parentIDHex, "ownerId": ownerID})
	if err != nil {
		return err
	}
	defer folderCursor.Close(ctx)

	for folderCursor.Next(ctx) {
		var subfolder models.Folder
		if err := folderCursor.Decode(&subfolder); err != nil {
			return err
		}
		if err := deleteFolderRecursively(ctx, subfolder.ID.Hex(), ownerID); err != nil {
			return err
		}
		_, err = foldersCollection.DeleteOne(ctx, bson.M{"_id": subfolder.ID, "ownerId": ownerID})
		if err != nil {
			return err
		}
	}
	return nil
}
