// ./cogniscan-backend/internal/services/mega_service.go
package services

import (
	"fmt"
	"log"
	"os"

	"github.com/t3rm1n4l/go-mega"
)

var megaClient *mega.Mega

// InitMegaService reads credentials from .env, logs into MEGA,
// and stores the client for global use.
func InitMegaService() error {
	log.Println("[MegaService] Initializing...")
	email := os.Getenv("MEGA_EMAIL")
	password := os.Getenv("MEGA_PASSWORD")

	if email == "" || password == "" {
		return fmt.Errorf("MEGA_EMAIL and MEGA_PASSWORD must be set in .env file")
	}

	m := mega.New()
	err := m.Login(email, password)
	if err != nil {
		return fmt.Errorf("failed to log into MEGA: %v", err)
	}

	megaClient = m
	log.Println("[MegaService] Successfully logged into MEGA.")
	return nil
}

// GetClient returns the initialized MEGA client.
func GetClient() *mega.Mega {
	return megaClient
}

// FindOrCreateCogniScanNode finds the "CogniScan" folder in the MEGA root,
// or creates it if it doesn't exist.
func FindOrCreateCogniScanNode(m *mega.Mega) (*mega.Node, error) {
	nodes, err := m.FS.GetChildren(m.FS.GetRoot())
	if err != nil {
		return nil, fmt.Errorf("could not get root children from MEGA: %v", err)
	}

	// Search for an existing "CogniScan" folder
	for _, node := range nodes {
		if node.GetType() == mega.FOLDER && node.GetName() == "CogniScan" {
			log.Println("[MegaService] Found existing 'CogniScan' folder in MEGA.")
			return node, nil
		}
	}

	// If not found, create it
	log.Println("[MegaService] 'CogniScan' folder not found. Creating it now...")
	cogniScanNode, err := m.CreateDir("CogniScan", m.FS.GetRoot())
	if err != nil {
		return nil, fmt.Errorf("failed to create 'CogniScan' folder in MEGA: %v", err)
	}

	log.Println("[MegaService] Successfully created 'CogniScan' folder.")
	return cogniScanNode, nil
}
