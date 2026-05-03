package handlers

import (
	"context"
	"net/http"
	"os"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"
	"cogniscan/backend/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// FolderMasteryResponse represents the folder mastery response (legacy)
type FolderMasteryResponse struct {
	FolderID       string    `json:"folderId"`
	UserID         string    `json:"userId"`
	TotalNotes     int       `json:"totalNotes"`
	MasteredNotes  int       `json:"masteredNotes"`
	LearntNotes    int       `json:"learntNotes"`
	MasteryLevel   string    `json:"masteryLevel"`
	MasteryPercent float64   `json:"masteryPercent"`
	LastStudyDate  time.Time `json:"lastStudyDate"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// MasteryUpdateRequest represents a mastery update request
type MasteryUpdateRequest struct {
	NoteID    string `json:"noteId" binding:"required"`
	IsCorrect bool   `json:"isCorrect" binding:"required"`
}

// GetFolderMastery is a deprecated wrapper for backward compatibility
// @Deprecated Use GetNodeMastery with node ID instead
func GetFolderMastery(c *gin.Context) {
	folderID := c.Param("folderId")
	userID := c.GetString("userId")
	if folderID == "" || userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "folderId and userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get the folder as a node
	node, err := services.GetNodeByID(ctx, folderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	// Verify ownership
	if node.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Verify it's a folder
	if node.Metadata.Type != models.NodeTypeFolder {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not a folder node"})
		return
	}

	// Return mastery (it's now embedded in the node)
	response := FolderMasteryResponse{
		FolderID:       folderID,
		UserID:         userID,
		TotalNotes:     node.Mastery.TotalNotes,
		MasteredNotes:  node.Mastery.MasteredNotes,
		LearntNotes:    node.Mastery.LearntNotes,
		MasteryLevel:   node.Mastery.MasteryLevel,
		MasteryPercent: node.Mastery.MasteryPercent,
		LastStudyDate:  node.Mastery.LastStudyDate,
		CreatedAt:      node.CreatedAt,
		UpdatedAt:      node.UpdatedAt,
	}

	c.JSON(http.StatusOK, response)
}

// GetAllFoldersMastery is a deprecated wrapper for backward compatibility
// @Deprecated Use GetAllNodesMastery instead
func GetAllFoldersMastery(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get all folder nodes owned by user
	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	cursor, err := nodesCollection.Find(ctx, bson.M{"ownerId": userID, "metadata.type": models.NodeTypeFolder})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch folders"})
		return
	}
	defer cursor.Close(ctx)

	var nodes []models.Node
	if err := cursor.All(ctx, &nodes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode folders"})
		return
	}

	// Build response with mastery data
	foldersMastery := []FolderMasteryResponse{}
	for _, node := range nodes {
		foldersMastery = append(foldersMastery, FolderMasteryResponse{
			FolderID:       node.ID.Hex(),
			UserID:         node.OwnerID,
			TotalNotes:     node.Mastery.TotalNotes,
			MasteredNotes:  node.Mastery.MasteredNotes,
			LearntNotes:    node.Mastery.LearntNotes,
			MasteryLevel:   node.Mastery.MasteryLevel,
			MasteryPercent: node.Mastery.MasteryPercent,
			LastStudyDate:  node.Mastery.LastStudyDate,
			CreatedAt:      node.CreatedAt,
			UpdatedAt:      node.Mastery.LastUpdated,
		})
	}

	response := GetAllFoldersMasteryResponse{
		Folders: foldersMastery,
		Total:   len(foldersMastery),
	}

	c.JSON(http.StatusOK, response)
}

type GetAllFoldersMasteryResponse struct {
	Folders []FolderMasteryResponse `json:"folders"`
	Total   int                     `json:"total"`
}

// UpdateNoteMastery is a deprecated wrapper for backward compatibility
// @Deprecated Use ReviewNoteNode in node_handler instead
func UpdateNoteMastery(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId required"})
		return
	}

	var req MasteryUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify the note node exists and belongs to user
	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	var node models.Node
	err := nodesCollection.FindOne(ctx, bson.M{"_id": req.NoteID, "ownerId": userID}).Decode(&node)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	if node.Metadata.Type != models.NodeTypeNote {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Can only review note nodes"})
		return
	}

	// Update the note review (this also updates the node's mastery)
	err = services.UpdateNoteReview(ctx, req.NoteID, userID, req.IsCorrect)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update mastery"})
		return
	}

	// Enqueue mastery update for ancestors
	services.EnqueueAncestorMasteryUpdate(req.NoteID)

	c.JSON(http.StatusOK, gin.H{
		"message":   "Mastery updated",
		"noteId":    req.NoteID,
		"isCorrect": req.IsCorrect,
	})
}

// GetNodeMastery returns mastery status for a node
// @Summary Returns mastery information including total notes, mastered count, and mastery percentage
func GetNodeMastery(c *gin.Context) {
	nodeID := c.Param("nodeId")
	userID := c.GetString("userId")
	if nodeID == "" || userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nodeId and userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get the node
	node, err := services.GetNodeByID(ctx, nodeID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	// Verify ownership
	if node.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Return mastery (it's now embedded in the node)
	response := gin.H{
		"nodeId":        nodeID,
		"totalNotes":     node.Mastery.TotalNotes,
		"masteredNotes":  node.Mastery.MasteredNotes,
		"learntNotes":    node.Mastery.LearntNotes,
		"masteryLevel":   node.Mastery.MasteryLevel,
		"masteryPercent": node.Mastery.MasteryPercent,
		"lastStudyDate":  node.Mastery.LastStudyDate,
		"lastUpdated":    node.Mastery.LastUpdated,
	}

	c.JSON(http.StatusOK, response)
}

// GetAllNodesMastery returns mastery for all user nodes
// @Summary Returns mastery data for all nodes belonging to the user
func GetAllNodesMastery(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get all nodes owned by user
	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	cursor, err := nodesCollection.Find(ctx, bson.M{"ownerId": userID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch nodes"})
		return
	}
	defer cursor.Close(ctx)

	var nodes []models.Node
	if err := cursor.All(ctx, &nodes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode nodes"})
		return
	}

	// Build response with mastery data
	nodesMastery := []gin.H{}
	for _, node := range nodes {
		// Only include nodes that have mastery data (folders and notes)
		nodesMastery = append(nodesMastery, gin.H{
			"nodeId":        node.ID.Hex(),
			"type":          node.Metadata.Type,
			"name":          node.Name,
			"parentId":      node.ParentID,
			"totalNotes":     node.Mastery.TotalNotes,
			"masteredNotes":  node.Mastery.MasteredNotes,
			"learntNotes":    node.Mastery.LearntNotes,
			"masteryLevel":   node.Mastery.MasteryLevel,
			"masteryPercent": node.Mastery.MasteryPercent,
			"lastStudyDate":  node.Mastery.LastStudyDate,
			"lastUpdated":    node.Mastery.LastUpdated,
		})
	}

	response := gin.H{
		"nodes": nodesMastery,
		"total": len(nodesMastery),
	}

	c.JSON(http.StatusOK, response)
}

// RefreshNodeMastery forces a mastery recalculation for a node and its ancestors
// @Summary Forces mastery refresh for a node (useful for debugging or manual sync)
func RefreshNodeMastery(c *gin.Context) {
	nodeID := c.Param("nodeId")
	userID := c.GetString("userId")
	if nodeID == "" || userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nodeId and userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify ownership
	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	var node models.Node
	err := nodesCollection.FindOne(ctx, bson.M{"_id": nodeID, "ownerId": userID}).Decode(&node)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}

	// Update mastery for this node
	err = services.UpdateNodeMastery(ctx, nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refresh mastery"})
		return
	}

	// Trigger background update for ancestors (if not root)
	if node.ParentID != "" {
		services.EnqueueAncestorMasteryUpdate(nodeID)
	}

	// Get updated node
	updatedNode, err := services.GetNodeByID(ctx, nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch updated node"})
		return
	}

	response := gin.H{
		"nodeId":        nodeID,
		"totalNotes":     updatedNode.Mastery.TotalNotes,
		"masteredNotes":  updatedNode.Mastery.MasteredNotes,
		"learntNotes":    updatedNode.Mastery.LearntNotes,
		"masteryLevel":   updatedNode.Mastery.MasteryLevel,
		"masteryPercent": updatedNode.Mastery.MasteryPercent,
		"lastStudyDate":  updatedNode.Mastery.LastStudyDate,
		"lastUpdated":    updatedNode.Mastery.LastUpdated,
	}

	c.JSON(http.StatusOK, response)
}

// GetMasteryStats returns aggregate mastery statistics for the user
// @Summary Returns overall mastery statistics across all nodes
func GetMasteryStats(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get all nodes owned by user
	nodesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
	cursor, err := nodesCollection.Find(ctx, bson.M{"ownerId": userID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch nodes"})
		return
	}
	defer cursor.Close(ctx)

	var nodes []models.Node
	if err := cursor.All(ctx, &nodes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode nodes"})
		return
	}

	// Calculate aggregate stats
	var (
		totalNotes     int
		masteredNotes  int
		learntNotes    int
		totalFolders   int
		totalNoteNodes int
	)

	for _, node := range nodes {
		if node.Metadata.Type == models.NodeTypeFolder {
			totalFolders++
		} else if node.Metadata.Type == models.NodeTypeNote {
			totalNoteNodes++
		}

		// Use mastery from each node (for folders, this aggregates children)
		totalNotes += node.Mastery.TotalNotes
		masteredNotes += node.Mastery.MasteredNotes
		learntNotes += node.Mastery.LearntNotes
	}

	// Calculate overall mastery level
	overallMasteryPercent := 0.0
	if totalNotes > 0 {
		overallMasteryPercent = float64(masteredNotes) / float64(totalNotes)
	}
	overallMasteryLevel := determineMasteryLevel(overallMasteryPercent)

	response := gin.H{
		"totalNodes":     len(nodes),
		"totalFolders":    totalFolders,
		"totalNoteNodes":  totalNoteNodes,
		"totalNotes":     totalNotes,
		"masteredNotes":  masteredNotes,
		"learntNotes":    learntNotes,
		"masteryLevel":   overallMasteryLevel,
		"masteryPercent": overallMasteryPercent,
	}

	c.JSON(http.StatusOK, response)
}

// determineMasteryLevel returns the mastery level based on percentage
func determineMasteryLevel(percent float64) string {
	if percent > 0.8 {
		return "Mastered"
	} else if percent >= 0.5 {
		return "Learnt"
	}
	return "Review Soon"
}
