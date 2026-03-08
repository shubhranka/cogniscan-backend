package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHealthCheck(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create a test router
	router := gin.New()
	router.GET("/health", HealthCheck)

	// Create a test request
	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a response recorder
	w := httptest.NewRecorder()

	// Serve the request
	router.ServeHTTP(w, req)

	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("HealthCheck() status = %v, want %v", w.Code, http.StatusOK)
	}

	// Check response body
	expectedBody := `{"message":"OK"}`
	if w.Body.String() != expectedBody {
		t.Errorf("HealthCheck() body = %v, want %v", w.Body.String(), expectedBody)
	}

	// Check Content-Type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json; charset=utf-8" && contentType != "application/json" {
		t.Errorf("HealthCheck() Content-Type = %v, want application/json", contentType)
	}
}

func TestHealthCheckWithMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/health", HealthCheck)

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req, err := http.NewRequest(method, "/health", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if method == "GET" {
				if w.Code != http.StatusOK {
					t.Errorf("GET method status = %v, want %v", w.Code, http.StatusOK)
				}
			} else {
				// Other methods should be handled by Gin's default 404/405
				// We just verify the handler runs without panic
			}
		})
	}
}
