package handlers

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/models"
	"cogniscan/backend/internal/queue"
	"cogniscan/backend/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// CreateNodePayload defines the expected JSON for creating a node
type CreateNodePayload struct {
	Name     string          `json:"name" binding:"required"`
	Type     models.NodeType `json:"type" binding:"required"`
	ParentID string          `json:"parentId"`          // Can be empty for root nodes
	DriveID  string          `json:"driveId,omitempty"` // Only for note type
}

// CreateNode creates a new node (folder or note)
func CreateNode(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var payload CreateNodePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	// Validate node type
	if payload.Type != models.NodeTypeFolder && payload.Type != models.NodeTypeNote {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node type, must be 'folder' or 'note'"})
		return
	}

	// Prevent self-referencing parentId
	if payload.ParentID != "" && payload.ParentID == "new" {
		// This is a placeholder - in some UI flows "new" might be used
		// If so, you may want to handle this differently
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid parent ID"})
		return
	}

	// If parentId is provided, verify it exists and is not the same as the node being created
	if payload.ParentID != "" {
		parentObjectID, err := primitive.ObjectIDFromHex(payload.ParentID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid parent ID format"})
			return
		}

		// Verify parent exists and user owns it
		nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
		var parent models.Node
		verifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = nodesCollection.FindOne(verifyCtx, bson.M{
			"_id":      parentObjectID,
			"ownerId":  firebaseUser.Claims["email"].(string),
			"parentId": bson.M{"$ne": payload.ParentID}, // Parent is not self-referencing
		}).Decode(&parent)
		cancel()

		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Parent node not found or is self-referencing"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify parent node"})
			}
			return
		}

		// Verify parent is a folder
		if parent.Metadata.Type != models.NodeTypeFolder {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Parent must be a folder"})
			return
		}
	}

	// For notes, validate driveId is present
	if payload.Type == models.NodeTypeNote && payload.DriveID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "driveId is required for note nodes"})
		return
	}

	now := time.Now()
	nodeID := primitive.NewObjectID()

	// Initialize mastery for the new node
	mastery := models.NodeMastery{
		TotalNotes:     0,
		MasteredNotes:  0,
		LearntNotes:    0,
		MasteryLevel:   "Review Soon",
		MasteryPercent: 0,
		LastStudyDate:  now,
		LastUpdated:    now,
	}

	metadata := models.NodeMetadata{
		Type: payload.Type,
	}

	if payload.Type == models.NodeTypeNote {
		metadata.DriveID = payload.DriveID
		mastery.TotalNotes = 1
	}

	newNode := models.Node{
		ID:             nodeID,
		Name:           payload.Name,
		ParentID:       payload.ParentID,
		Children:       []string{},
		TotalNoteCount: 0,
		Metadata:       metadata,
		OwnerID:        firebaseUser.Claims["email"].(string),
		CreatedAt:      now,
		UpdatedAt:      now,
		Mastery:        mastery,
	}

	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := nodesCollection.InsertOne(ctx, newNode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create node"})
		return
	}

	// Add this node to parent's children array
	if payload.ParentID != "" {
		_, err = nodesCollection.UpdateOne(ctx,
			bson.M{"_id": payload.ParentID, "ownerId": firebaseUser.Claims["email"]},
			bson.M{"$push": bson.M{"children": nodeID.Hex()}, "$set": bson.M{"updatedAt": now}})
		if err != nil {
			log.Printf("[CreateNode] Failed to add node to parent's children: %v", err)
		}

		// Update parent's totalNoteCount
		if payload.Type == models.NodeTypeNote {
			// Update TotalNoteCount for all ancestors
			updateAncestorTotalNoteCount(ctx, payload.ParentID, firebaseUser.Claims["email"].(string), 1)

			// Enqueue mastery update for ancestors
			services.EnqueueAncestorMasteryUpdate(nodeID.Hex())
		}
	}

	c.JSON(http.StatusCreated, newNode)
}

// GetNode retrieves a single node by ID
func GetNode(c *gin.Context) {
	nodeID := c.Param("id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Node ID required"})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	node, err := services.GetNodeByID(ctx, nodeID)
	if err != nil {
		if err == services.ErrNodeNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch node"})
		}
		return
	}

	// Verify ownership
	if node.OwnerID != firebaseUser.Claims["email"].(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, node)
}

// GetNodeChildren retrieves all child nodes for a given parent
func GetNodeChildren(c *gin.Context) {
	parentID := c.Query("parentId")
	if parentID == "root" {
		parentID = ""
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	children, err := services.GetNodesByParent(ctx, parentID, firebaseUser.Claims["email"].(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch child nodes"})
		return
	}

	if children == nil {
		children = []models.Node{}
	}

	c.JSON(http.StatusOK, children)
}

// GetNodeTree retrieves a node and all its descendants
func GetNodeTree(c *gin.Context) {
	nodeID := c.Param("id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Node ID required"})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	root, descendants, err := services.GetNodeTree(ctx, nodeID, 10)
	if err != nil {
		if err == services.ErrNodeNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch node tree"})
		}
		return
	}

	// Verify ownership
	if root.OwnerID != firebaseUser.Claims["email"].(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"root":        root,
		"descendants": descendants,
	})
}

// UpdateNodePayload defines the expected JSON for updating a node
type UpdateNodePayload struct {
	Name string `json:"name" binding:"required"`
}

// UpdateNode updates a node's name
func UpdateNode(c *gin.Context) {
	nodeID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node ID"})
		return
	}

	var payload UpdateNodePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	filter := bson.M{"_id": nodeID, "ownerId": firebaseUser.Claims["email"]}
	update := bson.M{"$set": bson.M{"name": payload.Name, "updatedAt": time.Now()}}

	result, err := nodesCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update node"})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found or you don't have permission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Node updated successfully"})
}

// DeleteNode deletes a node and all its descendants
func DeleteNode(c *gin.Context) {
	nodeIDHex := c.Param("id")
	nodeID, err := primitive.ObjectIDFromHex(nodeIDHex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node ID"})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	userID := firebaseUser.Claims["email"].(string)

	// Get the node to check its type and parent
	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	var node models.Node
	err = nodesCollection.FindOne(ctx, bson.M{"_id": nodeID, "ownerId": userID}).Decode(&node)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	// Recursively delete all descendants
	deletedNoteCount := deleteNodeRecursively(ctx, nodeIDHex, userID)

	// Update parent's children array and totalNoteCount
	if node.ParentID != "" {
		_, err = nodesCollection.UpdateOne(ctx,
			bson.M{"_id": node.ParentID, "ownerId": userID},
			bson.M{"$pull": bson.M{"children": nodeIDHex}, "$set": bson.M{"updatedAt": time.Now()}})
		if err != nil {
			log.Printf("[DeleteNode] Failed to remove node from parent's children: %v", err)
		}

		// Update ancestor TotalNoteCounts
		updateAncestorTotalNoteCount(ctx, node.ParentID, userID, -deletedNoteCount)

		// Enqueue mastery update for ancestors
		services.EnqueueAncestorMasteryUpdate(node.ParentID)
	}

	// Finally delete the main node
	_, err = nodesCollection.DeleteOne(ctx, bson.M{"_id": nodeID, "ownerId": userID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete the main node"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Node deleted successfully"})
}

// deleteNodeRecursively deletes a node and all its descendants
// Returns the number of note nodes deleted
func deleteNodeRecursively(ctx context.Context, nodeIDHex, ownerID string) int {
	dbName := os.Getenv("DB_NAME")
	nodesCollection := database.Client.Database(dbName).Collection("nodes")

	// Find the node
	var node models.Node
	err := nodesCollection.FindOne(ctx, bson.M{"_id": nodeIDHex, "ownerId": ownerID}).Decode(&node)
	if err != nil {
		log.Printf("Failed to find node %s: %v", nodeIDHex, err)
		return 0
	}

	deletedNoteCount := 0

	// If it's a note, delete from Google Drive
	if node.Metadata.Type == models.NodeTypeNote && node.Metadata.DriveID != "" {
		err := services.DeleteFile(node.Metadata.DriveID)
		if err != nil {
			log.Printf("Failed to delete file from Drive: %v", err)
		}
		deletedNoteCount = 1
	}

	// Recursively delete all children
	for _, childID := range node.Children {
		deletedNoteCount += deleteNodeRecursively(ctx, childID, ownerID)

		// Delete child node
		_, err = nodesCollection.DeleteOne(ctx, bson.M{"_id": childID, "ownerId": ownerID})
		if err != nil {
			log.Printf("Failed to delete child node %s: %v", childID, err)
		}
	}

	return deletedNoteCount
}

// updateAncestorTotalNoteCount updates the totalNoteCount for all ancestors
// delta can be positive (add) or negative (remove)
func updateAncestorTotalNoteCount(ctx context.Context, parentID, ownerID string, delta int) {
	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")

	currentParentID := parentID
	for currentParentID != "" {
		_, err := nodesCollection.UpdateOne(ctx,
			bson.M{"_id": currentParentID, "ownerId": ownerID},
			bson.M{"$inc": bson.M{"totalNoteCount": delta}, "$set": bson.M{"updatedAt": time.Now()}})
		if err != nil {
			log.Printf("[updateAncestorTotalNoteCount] Failed to update node %s: %v", currentParentID, err)
			break
		}

		// Get parent of this node
		var node models.Node
		err = nodesCollection.FindOne(ctx, bson.M{"_id": currentParentID, "ownerId": ownerID}).Decode(&node)
		if err != nil {
			break
		}

		currentParentID = node.ParentID
	}
}

// CreateNoteNode creates a new note node with file upload
func CreateNoteNode(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err := c.Request.ParseMultipartForm(20 << 20)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error parsing form data"})
		return
	}

	name := c.Request.FormValue("name")
	parentID := c.Request.FormValue("parentId")
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image file is required"})
		return
	}
	defer file.Close()

	// Read image bytes for AI processing
	imageBytes, err := io.ReadAll(file)
	if err != nil {
		log.Printf("[NoteNodeHandler] Failed to read image bytes: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process image"})
		return
	}

	driveID, err := services.UploadFile(header.Filename, bytes.NewReader(imageBytes))
	if err != nil {
		log.Printf("[NoteNodeHandler] Failed to upload to Google Drive: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload file"})
		return
	}

	now := time.Now()
	nodeID := primitive.NewObjectID()

	mastery := models.NodeMastery{
		TotalNotes:     1,
		MasteredNotes:  0,
		LearntNotes:    0,
		MasteryLevel:   "Review Soon",
		MasteryPercent: 0,
		LastStudyDate:  now,
		LastUpdated:    now,
	}

	newNode := models.Node{
		ID:             nodeID,
		Name:           name,
		ParentID:       parentID,
		Children:       []string{},
		TotalNoteCount: 1,
		Metadata: models.NodeMetadata{
			Type:    models.NodeTypeNote,
			DriveID: driveID,
		},
		OwnerID:   firebaseUser.Claims["email"].(string),
		CreatedAt: now,
		UpdatedAt: now,
		Mastery:   mastery,
	}

	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = nodesCollection.InsertOne(ctx, newNode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save node record"})
		return
	}

	// Add to parent's children array
	if parentID != "" {
		_, err = nodesCollection.UpdateOne(ctx,
			bson.M{"_id": parentID, "ownerId": firebaseUser.Claims["email"]},
			bson.M{"$push": bson.M{"children": nodeID.Hex()}, "$set": bson.M{"updatedAt": now}})
		if err != nil {
			log.Printf("[NoteNodeHandler] Failed to add node to parent's children: %v", err)
		}

		// Update ancestor TotalNoteCounts
		updateAncestorTotalNoteCount(ctx, parentID, firebaseUser.Claims["email"].(string), 1)
	}

	// Enqueue caption generation job
	job := queue.CaptionJob{
		ID:      primitive.NewObjectID().Hex(),
		NoteID:  nodeID.Hex(),
		DriveID: driveID,
	}
	if err := services.EnqueueCaptionJob(job); err != nil {
		log.Printf("[NoteNodeHandler] Failed to enqueue caption job: %v", err)
	}

	c.JSON(http.StatusCreated, newNode)
}

// GetNodeImage serves a node's image as a secure proxy
func GetNodeImage(c *gin.Context) {
	nodeID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node ID"})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var node models.Node
	filter := bson.M{"_id": nodeID, "ownerId": firebaseUser.Claims["email"]}
	if err := nodesCollection.FindOne(ctx, filter).Decode(&node); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found or access denied"})
		return
	}

	if node.Metadata.Type != models.NodeTypeNote || node.Metadata.DriveID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No image associated with this node"})
		return
	}

	log.Printf("[GetNodeImage] Downloading DriveID: %s", node.Metadata.DriveID)
	resp, err := services.DownloadFileContent(node.Metadata.DriveID)
	if err != nil {
		log.Printf("[GetNodeImage] Error downloading from Drive: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve image from storage"})
		return
	}
	defer resp.Body.Close()

	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Header("Content-Length", resp.Header.Get("Content-Length"))
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		log.Printf("[GetNodeImage] Error streaming file: %v", err)
	}
}

// RegenerateNodeCaption regenerates caption for a note node
func RegenerateNodeCaption(c *gin.Context) {
	nodeID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node ID"})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var node models.Node
	filter := bson.M{"_id": nodeID, "ownerId": firebaseUser.Claims["email"]}
	if err := nodesCollection.FindOne(ctx, filter).Decode(&node); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found or access denied"})
		return
	}

	if node.Metadata.Type != models.NodeTypeNote || node.Metadata.DriveID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This node is not a note with an image"})
		return
	}

	job := queue.CaptionJob{
		ID:      primitive.NewObjectID().Hex(),
		NoteID:  nodeID.Hex(),
		DriveID: node.Metadata.DriveID,
	}

	if err := services.EnqueueCaptionJob(job); err != nil {
		log.Printf("[RegenerateNodeCaption] Failed to enqueue job: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue caption job"})
		return
	}

	log.Printf("[RegenerateNodeCaption] Enqueued job %s for node %s", job.ID, nodeID.Hex())
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Caption regeneration started",
		"nodeId":  nodeID.Hex(),
	})
}

// ReviewNoteNode reviews a note node, updating mastery
type ReviewNoteNodePayload struct {
	IsCorrect bool `json:"isCorrect" binding:"required"`
}

// ReviewNoteNode handles review of a note node
func ReviewNoteNode(c *gin.Context) {
	nodeID := c.Param("id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Node ID required"})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var payload ReviewNoteNodePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userID := firebaseUser.Claims["email"].(string)

	// Verify node exists and is a note
	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	var node models.Node
	err := nodesCollection.FindOne(ctx, bson.M{"_id": nodeID, "ownerId": userID}).Decode(&node)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	if node.Metadata.Type != models.NodeTypeNote {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Can only review note nodes"})
		return
	}

	// Update the note review
	err = services.UpdateNoteReview(ctx, nodeID, userID, payload.IsCorrect)
	if err != nil {
		log.Printf("[ReviewNoteNode] Failed to update review: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update review"})
		return
	}

	// Enqueue mastery update for ancestors
	services.EnqueueAncestorMasteryUpdate(nodeID)

	c.JSON(http.StatusOK, gin.H{
		"message": "Review recorded successfully",
		"nodeId":  nodeID,
	})
}

// GetNameSuggestionsForFolder returns AI-generated name suggestions for a folder node
func GetNameSuggestionsForFolder(c *gin.Context) {
	nodeID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node ID"})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userID := firebaseUser.Claims["email"].(string)

	// Verify node exists and is a folder
	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	var node models.Node
	err = nodesCollection.FindOne(ctx, bson.M{"_id": nodeID, "ownerId": userID}).Decode(&node)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	if node.Metadata.Type != models.NodeTypeFolder {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This endpoint is only for folder nodes"})
		return
	}

	// Get all note nodes under this folder (including nested subfolders)
	captionEmbeddingsCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("caption_embeddings")
	cursor, err := captionEmbeddingsCollection.Find(ctx, bson.M{
		"ownerId":  userID,
		"folderId": nodeID.Hex(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch captions"})
		return
	}
	defer cursor.Close(ctx)

	var captions []string
	if err := cursor.All(ctx, &captions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode captions"})
		return
	}

	if len(captions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No notes with captions found in folder"})
		return
	}

	suggestions, err := services.GenerateNameSuggestionsForFolder(captions)
	if err != nil {
		log.Printf("[GetNameSuggestionsForFolder] Failed to generate suggestions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate name suggestions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}

// GetNameSuggestionsForNote returns AI-generated name suggestions for a note node
func GetNameSuggestionsForNote(c *gin.Context) {
	nodeID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node ID"})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userID := firebaseUser.Claims["email"].(string)

	// Verify node exists and is a note
	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	var node models.Node
	err = nodesCollection.FindOne(ctx, bson.M{"_id": nodeID, "ownerId": userID}).Decode(&node)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	if node.Metadata.Type != models.NodeTypeNote {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This endpoint is only for note nodes"})
		return
	}

	// Get the caption for this note
	captionEmbeddingsCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("caption_embeddings")
	var captionEmbedding struct {
		Caption string `bson:"caption"`
	}
	err = captionEmbeddingsCollection.FindOne(ctx, bson.M{
		"noteId":  nodeID.Hex(),
		"ownerId": userID,
	}).Decode(&captionEmbedding)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No caption found for this note"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch caption"})
		}
		return
	}

	suggestions, err := services.GenerateNameSuggestionsForNote(captionEmbedding.Caption)
	if err != nil {
		log.Printf("[GetNameSuggestionsForNote] Failed to generate suggestions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate name suggestions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}
