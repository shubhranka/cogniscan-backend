package services

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestTokenFromFile(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		wantErr bool
	}{
		{
			name:    "Valid token file",
			file:    "test_token.json",
			// oauth2.Token uses lowercase field names in JSON
			content: `{"access_token":"test_token","refresh_token":"refresh_token","token_type":"Bearer"}`,
			wantErr: false,
		},
		{
			name:    "Invalid JSON",
			file:    "invalid_token.json",
			content: `invalid json`,
			wantErr: true,
		},
		{
			name:    "File not found",
			file:    "nonexistent.json",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.wantErr || tt.name == "Valid token file" || tt.name == "Invalid JSON" {
				// Create test file
				if tt.content != "" {
					err := os.WriteFile(tt.file, []byte(tt.content), 0600)
					if err != nil {
						t.Fatalf("Failed to create test file: %v", err)
					}
					defer os.Remove(tt.file)
				}
			}

			token, err := tokenFromFile(tt.file)

			if (err != nil) != tt.wantErr {
				t.Errorf("tokenFromFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && token == nil {
				t.Error("tokenFromFile() returned nil token")
			}

			if !tt.wantErr && token != nil {
				if token.AccessToken != "test_token" {
					t.Errorf("Expected AccessToken 'test_token', got '%s'", token.AccessToken)
				}
				if token.RefreshToken != "refresh_token" {
					t.Errorf("Expected RefreshToken 'refresh_token', got '%s'", token.RefreshToken)
				}
			}
		})
	}
}

func TestSaveToken(t *testing.T) {
	tests := []struct {
		name    string
		token   *oauth2.Token
		wantErr bool
	}{
		{
			name: "Save valid token",
			token: &oauth2.Token{
				AccessToken:  "test_token",
				RefreshToken: "refresh_token",
				TokenType:    "Bearer",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := "test_save_token.json"
			defer os.Remove(testFile)

			saveToken(testFile, tt.token)

			// Verify file was created and has content
			content, err := os.ReadFile(testFile)
			if err != nil {
				t.Errorf("Failed to read saved token file: %v", err)
				return
			}

			if len(content) == 0 {
				t.Error("Saved token file is empty")
			}

			if !strings.Contains(string(content), "test_token") {
				t.Error("Saved token file doesn't contain expected content")
			}
		})
	}
}

func TestInitDriveService(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		wantErr     bool
		description string
	}{
		{
			name:        "Missing environment variable",
			envValue:    "",
			wantErr:     true,
			description: "Should return error when COGNI_BACKEND is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("COGNI_BACKEND", tt.envValue)
				defer os.Unsetenv("COGNI_BACKEND")
			} else {
				os.Unsetenv("COGNI_BACKEND")
			}

			err := InitDriveService()

			if (err != nil) != tt.wantErr {
				t.Errorf("InitDriveService() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && err == nil {
				t.Error(tt.description)
			}
		})
	}
}

func TestGetDriveClient(t *testing.T) {
	client := GetDriveClient()

	// Initially, drive service is not initialized
	if client != nil && client != driveService {
		t.Error("GetDriveClient() should return nil or the initialized service")
	}
}

func TestUploadFile(t *testing.T) {
	tests := []struct {
		name    string
		setup   func()
		wantErr bool
	}{
		{
			name: "Service not initialized",
			setup: func() {
				// Ensure driveService is nil
				driveService = nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			// Save original state
			originalService := driveService
			defer func() { driveService = originalService }()

			_, err := UploadFile("test.jpg", strings.NewReader("test content"))

			if (err != nil) != tt.wantErr {
				t.Errorf("UploadFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDownloadFileContent(t *testing.T) {
	tests := []struct {
		name    string
		setup   func()
		wantErr bool
	}{
		{
			name: "Service not initialized",
			setup: func() {
				driveService = nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			// Save original state
			originalService := driveService
			defer func() { driveService = originalService }()

			_, err := DownloadFileContent("test-file-id")

			if (err != nil) != tt.wantErr {
				t.Errorf("DownloadFileContent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteFile(t *testing.T) {
	tests := []struct {
		name    string
		setup   func()
		wantErr bool
	}{
		{
			name: "Service not initialized",
			setup: func() {
				driveService = nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			// Save original state
			originalService := driveService
			defer func() { driveService = originalService }()

			err := DeleteFile("test-file-id")

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

