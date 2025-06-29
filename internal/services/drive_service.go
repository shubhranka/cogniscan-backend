// ./cogniscan-backend/internal/services/drive_service.go
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var driveService *drive.Service

// InitDriveService initializes the Google Drive client using credentials
// stored in an environment variable.
func InitDriveService() error {
	log.Println("[DriveService] Initializing...")
	ctx := context.Background()

	// --- UPDATED: Read credentials from environment variable ---
	credentialsJSONString := os.Getenv("COGNI_BACKEND")
	if credentialsJSONString == "" {
		return fmt.Errorf("COGNI_BACKEND environment variable not set")
	}

	// Convert the string to a byte slice
	var credentialsJSON map[string]interface{}
	err := json.Unmarshal([]byte(credentialsJSONString), &credentialsJSON)
	if err != nil {
		return fmt.Errorf("unable to rectify client secret from env to config: %v", err)
	}
	credentialsJSON["private_key"] = strings.ReplaceAll(credentialsJSON["private_key"].(string), "\\n", "\n")
	rectifiedCredentialsJSONString, err := json.Marshal(credentialsJSON)
	if err != nil {
		return fmt.Errorf("unable to rectify client secret from env to config: %v", err)
	}
	// --- END UPDATED SECTION ---

	config, err := google.JWTConfigFromJSON(rectifiedCredentialsJSONString, drive.DriveFileScope)
	if err != nil {
		return fmt.Errorf("unable to rectify client secret from env to config: %v", err)
	}
	client := config.Client(ctx)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("unable to retrieve Drive client: %v", err)
	}

	driveService = srv
	log.Println("[DriveService] Successfully initialized Google Drive client from environment variable.")
	return nil
}

// GetDriveClient returns the initialized Drive client.
func GetDriveClient() *drive.Service {
	return driveService
}

// UploadFile now uploads a PRIVATE file. It no longer returns a public URL.
func UploadFile(name string, content io.Reader) (string, error) {
	srv := GetDriveClient()
	if srv == nil {
		return "", fmt.Errorf("Drive service is not initialized")
	}
	folderID := os.Getenv("GOOGLE_DRIVE_FOLDER_ID")

	fileMetadata := &drive.File{
		Name:    name,
		Parents: []string{folderID},
	}

	file, err := srv.Files.Create(fileMetadata).Media(content).Do()
	if err != nil {
		return "", fmt.Errorf("could not create file: %v", err)
	}

	log.Printf("[DriveService] Private file uploaded successfully. ID: %s", file.Id)
	return file.Id, nil
}

// DownloadFileContent downloads the content of a private file.
func DownloadFileContent(fileID string) (*http.Response, error) {
	srv := GetDriveClient()
	if srv == nil {
		return nil, fmt.Errorf("Drive service is not initialized")
	}
	// Files.Get with Media option returns the file content
	return srv.Files.Get(fileID).Download()
}

// DeleteFile deletes a file from Google Drive using its ID.
func DeleteFile(fileID string) error {
	srv := GetDriveClient()
	if srv == nil {
		return fmt.Errorf("Drive service is not initialized")
	}
	err := srv.Files.Delete(fileID).Do()
	if err != nil {
		log.Printf("[DriveService] Failed to delete file %s: %v", fileID, err)
		return err
	}
	log.Printf("[DriveService] Successfully deleted file %s.", fileID)
	return nil
}
