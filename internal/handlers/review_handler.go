package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/models"
	"cogniscan/backend/internal/services"
)

type ReviewItem struct {
	ReviewID     string  `json:"reviewId"`
	NoteID       string  `json:"noteId"`
	NoteName     string  `json:"noteName"`
	PublicURL    string  `json:"publicUrl"`
	EaseFactor   float32 `json:"easeFactor"`
	Interval     int     `json:"interval"`
	Repetitions  int     `json:"repetitions"`
	NextReview   string  `json:"nextReview"`
	ToReview     bool    `json:"toReview"`
	TotalReviews int     `json:"totalReviews"`
	SuccessRate  float64 `json:"successRate"`
}

// GetReviewQueue returns notes due for review
func GetReviewQueue(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	reviews, err := services.GetReviewQueue(c.Request.Context(), firebaseUser.Claims["email"].(string), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get review queue"})
		return
	}

	// Get note details for each review
	noteIDs := make([]string, len(reviews))
	for i, r := range reviews {
		noteIDs[i] = r.NoteID
	}

	notes, err := services.GetNotesByIDs(c.Request.Context(), noteIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get note details"})
		return
	}

	// Create map of note ID to note
	noteMap := make(map[string]models.Note)
	for _, note := range notes {
		noteMap[note.ID.Hex()] = note
	}

	// Build response
	reviewItems := make([]ReviewItem, 0, len(reviews))
	for _, review := range reviews {
		note, ok := noteMap[review.NoteID]
		if !ok {
			continue
		}
		item := ReviewItem{
			ReviewID:     review.ID.Hex(),
			NoteID:       review.NoteID,
			NoteName:     note.Name,
			PublicURL:    note.PublicURL,
			EaseFactor:   review.EaseFactor,
			Interval:     review.Interval,
			Repetitions:  review.Repetitions,
			NextReview:   review.NextReview.Format(time.RFC3339),
			ToReview:     review.ToReview,
			TotalReviews: review.TotalReviews,
			SuccessRate:  0.0,
		}
		if review.TotalReviews > 0 {
			item.SuccessRate = float64(review.CorrectCount) / float64(review.TotalReviews) * 100
		}
		reviewItems = append(reviewItems, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"reviews": reviewItems,
		"total":   len(reviewItems),
	})
}

// GetNoteReviewHistory returns review history for a specific note
func GetNoteReviewHistory(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	noteID := c.Param("noteId")

	review, err := services.GetNoteReviewHistory(c.Request.Context(), noteID, firebaseUser.Claims["email"].(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Review not found"})
		return
	}

	c.JSON(http.StatusOK, review)
}

// UpdateReviewStatus clears the toReview flag when a review is opened
func UpdateReviewStatus(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	noteID := c.Param("noteId")
	if noteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Note ID is required"})
		return
	}

	err := services.UpdateReviewStatus(c.Request.Context(), noteID, firebaseUser.Claims["email"].(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update review status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Review status updated"})
}
