package main

import (
    "bufio"
    "fmt"
    "log"
    "net"
    "os"
    "time"
    "github.com/joho/godotenv"
)

// Load .env file and get the server host and port
func loadEnvVariables() (string, string) {
    err := godotenv.Load(".env")
    if err != nil {
        log.Fatalf("Error loading .env file: %v", err)
    }
    serverHost := os.Getenv("SERVER_HOST")
    serverPort := os.Getenv("SERVER_PORT")
    if serverHost == "" || serverPort == "" {
        log.Fatalf("SERVER_HOST or SERVER_PORT not set in .env file")
    }
    return serverHost, serverPort
}

// Helper function to continuously listen for server messages and echo them
func listenForMessages(conn net.Conn) {
    scanner := bufio.NewScanner(conn)
    for scanner.Scan() {
        message := scanner.Text()
        fmt.Println("Received from server:", message)
    }

    // Handle error when scanner stops (usually EOF/connection closed)
    if scanner.Err() != nil {
        log.Printf("Connection error: %v", scanner.Err())
    } else {
        log.Println("Server disconnected.")
    }
}

// Establish connection to the server with retry logic
func connectToServer(serverHost string, serverPort string) net.Conn {
    for {
        address := fmt.Sprintf("%s:%s", serverHost, serverPort)
        log.Printf("Attempting to connect to Surge Protocol server at %s...", address)
        conn, err := net.DialTimeout("tcp", address, 5 * time.Second)
        if err != nil {
            log.Printf("Failed to connect to server: %v", err)
            log.Println("Retrying in 5 seconds...")
            time.Sleep(5 * time.Second)
            continue // Retry connection
        }
        log.Println("Connected to Surge Protocol server.")
        return conn
    }
}

func main() {
    // Load environment variables for server host and port
    serverHost, serverPort := loadEnvVariables()

    for {
        conn := connectToServer(serverHost, serverPort) // Attempt connection
        listenForMessages(conn)                         // Listen for protocol messages

        // If we reach here, the connection was lost; retry connection
        conn.Close()
        log.Println("Connection lost. Reconnecting...")
    }
}