package main

import (
	"log"
	"os"

	"github.com/iamangus/vitra-mcp/internal"
	"github.com/iamangus/vitra-mcp/internal/vector"
)

func main() {
	vaultPath := os.Getenv("VAULT_PATH")
	if vaultPath == "" {
		vaultPath = "./vault"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	// Initialize vector store (ChromaDB)
	vectorStore := vector.NewChromaStore()
	defer vectorStore.Close()

	// Initialize vault with vector store
	vault := internal.NewVault(vaultPath, vectorStore)

	log.Printf("Vitra MCP server starting on :%s with vault at %s", port, vaultPath)
	if err := internal.StartMCPServer(vault, port); err != nil {
		log.Fatal(err)
	}
}
