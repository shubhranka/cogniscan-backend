package services

import (
	"os"
	"testing"
)

func TestInitAIService(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		wantErr bool
	}{
		{
			name:    "Init with API key",
			apiKey:  "test-api-key-123",
			wantErr: false,
		},
		{
			name:    "Init without API key",
			apiKey:  "",
			wantErr: false, // Gracefully handles missing key
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variable
			if tt.apiKey != "" {
				os.Setenv("NVIDIA_API_KEY", tt.apiKey)
				defer os.Unsetenv("NVIDIA_API_KEY")
			} else {
				os.Unsetenv("NVIDIA_API_KEY")
			}

			err := InitAIService()

			if (err != nil) != tt.wantErr {
				t.Errorf("InitAIService() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateCaption(t *testing.T) {
	tests := []struct {
		name      string
		setup     func()
		imageData []byte
		wantErr   bool
	}{
		{
			name: "AI client not initialized",
			setup: func() {
				os.Unsetenv("NVIDIA_API_KEY")
			},
			imageData: []byte("fake image data"),
			wantErr:   true,
		},
		{
			name: "Empty image data",
			setup: func() {
				os.Setenv("NVIDIA_API_KEY", "test-api-key")
			},
			imageData: []byte{},
			wantErr:   true,
		},
		{
			name: "Nil image data",
			setup: func() {
				os.Setenv("NVIDIA_API_KEY", "test-api-key")
			},
			imageData: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			caption, err := GenerateCaption(tt.imageData)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateCaption() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && caption == "" {
				t.Error("GenerateCaption() returned empty caption")
			}
		})
	}
}

func TestIsClientInitialized(t *testing.T) {
	tests := []struct {
		name      string
		setup     func()
		wantInit  bool
	}{
		{
			name: "Client initialized",
			setup: func() {
				os.Setenv("NVIDIA_API_KEY", "test-api-key")
			},
			wantInit: true,
		},
		{
			name: "Client not initialized",
			setup: func() {
				os.Unsetenv("NVIDIA_API_KEY")
			},
			wantInit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			// This is an internal function, so we test through GenerateCaption
			caption, err := GenerateCaption([]byte("test"))

			if tt.wantInit {
				if err != nil && err.Error() == "AI client not initialized" {
					t.Errorf("Expected AI client to be initialized")
				}
			} else {
				if err == nil {
					t.Error("Expected error when client not initialized")
				}
				if caption != "" {
					t.Errorf("Expected empty caption, got %s", caption)
				}
			}
		})
	}
}
