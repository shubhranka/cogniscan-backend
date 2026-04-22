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

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var driveService *drive.Service

// --- NEW HELPER FUNCTIONS FOR OAUTH 2.0 ---

// getClient retrieves a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	// Automatically refreshes the token when it expires using the refresh token
	return config.Client(context.Background(), tok)
}

// getTokenFromWeb requests a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("\n=======================================================\n")
	fmt.Printf("Go to the following link in your browser to authorize:\n\n%v\n\n", authURL)
	fmt.Printf("Type the authorization code here and press Enter: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// tokenFromFile retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// saveToken saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// --- END NEW HELPER FUNCTIONS ---

// InitDriveService initializes the Google Drive client using OAuth User Credentials.
func InitDriveService() error {
	log.Println("[DriveService] Initializing with Personal Account...")
	ctx := context.Background()

	// COGNI_BACKEND must now contain OAUTH CLIENT ID JSON, not Service Account JSON
	credentialsJSONString := os.Getenv("COGNI_BACKEND")
	if credentialsJSONString == "" {
		return fmt.Errorf("COGNI_BACKEND environment variable not set")
	}

	// // Use google.ConfigFromJSON instead of JWTConfigFromJSON
	// config, err := google.ConfigFromJSON([]byte(credentialsJSONString), drive.DriveFileScope)
	// if err != nil {
	// 	return fmt.Errorf("unable to parse client secret to config: %v", err)
	// }

	// // This handles the interactive login and creates/reads token.json
	// client := getClient(config)

	// srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	srv, err := drive.NewService(ctx, option.WithCredentialsFile("service-account.json"))
	if err != nil {
		return fmt.Errorf("unable to retrieve Drive client: %v", err)
	}

	driveService = srv
	log.Println("[DriveService] Successfully initialized Google Drive client with Personal Account.")
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
