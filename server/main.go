package main

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net"
    "os"
    "sync"
    "time"

    "github.com/go-redis/redis/v8"
    "golang.org/x/net/context"
)

var (
    rdb     *redis.Client
    ctx     = context.Background()
    mu      sync.Mutex
    conns   = make(map[net.Conn]struct{}) // Use a thread-safe map for active connections
    config  Config                        // Global server configuration
)

// Config struct for reading JSON configuration
type Config struct {
    TickDuration int    `json:"tick_duration"` // Tick duration in seconds
    ServerPort   string `json:"server_port"`   // Port for the server to listen on
}

// Load configuration from the config.json file
func loadConfig() error {
    file, err := os.Open("config.json")
    if err != nil {
        return err
    }
    defer file.Close()

    byteValue, err := ioutil.ReadAll(file)
    if err != nil {
        return err
    }

    err = json.Unmarshal(byteValue, &config)
    if err != nil {
        return err
    }

    log.Printf("Configuration loaded: TickDuration = %d, ServerPort = %s", config.TickDuration, config.ServerPort)
    return nil
}

// Send tick message to all connected clients
func sendTickMessage(tick int) {
    mu.Lock()
    defer mu.Unlock()

    message := fmt.Sprintf("TICK %d\n", tick)
    log.Printf("Sending tick %d to %d clients.", tick, len(conns))

    for conn := range conns {
        _, err := conn.Write([]byte(message))
        if err != nil {
            log.Printf("Failed to send tick to client %v: %v. Closing connection.", conn.RemoteAddr(), err)
            conn.Close()
            delete(conns, conn)
        }
    }
}

// Game tick process - Sends "TICK X" every tick_duration seconds
func gameLoop() {
    tick := 0
    for {
        time.Sleep(time.Duration(config.TickDuration) * time.Second) // Use tick duration from configuration
        tick++
        log.Printf("Surge Protocol: Sending tick %d", tick)
        sendTickMessage(tick)

        // Simulate storing the tick count in Redis
        err := rdb.Set(ctx, "game:tick", tick, 0).Err()
        if err != nil {
            log.Println("Redis error:", err)
        }
    }
}

// Handle incoming client connections
func handleConnection(conn net.Conn) {
    log.Printf("New client connected: %v", conn.RemoteAddr())

    // Add connection to the set
    mu.Lock()
    conns[conn] = struct{}{}
    mu.Unlock()

    defer func() {
        conn.Close()
        mu.Lock()
        delete(conns, conn)
        mu.Unlock()
        log.Printf("Client disconnected: %v", conn.RemoteAddr())
    }()

    // We won't read data from the client, but keep the connection for tick messages
    for {
        buf := make([]byte, 1)
        _, err := conn.Read(buf)
        if err != nil {
            return // Client disconnected or error occurred
        }
    }
}

// Start the TCP server that listens for client connections
func startServer() {
    address := fmt.Sprintf(":%s", config.ServerPort)
    listener, err := net.Listen("tcp", address)
    if err != nil {
        log.Fatalf("Error starting Surge Protocol server: %v", err)
    }
    log.Printf("Surge Protocol server listening on port %s", config.ServerPort)

    for {
        conn, err := listener.Accept()
        if err != nil {
            log.Println("Error accepting connection:", err)
            continue
        }
        go handleConnection(conn) // Handle each connection in a new goroutine
    }
}

// Initialize Redis connection
func initRedis() {
    rdb = redis.NewClient(&redis.Options{
        Addr: "redis:6379", // Redis hostname set in docker-compose
    })
    if _, err := rdb.Ping(ctx).Result(); err != nil {
        log.Fatalf("Error connecting to Redis: %v", err)
    }
    log.Println("Surge Protocol connected to Redis.")
}

func main() {
    // Load configuration from config.json
    if err := loadConfig(); err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }

    go gameLoop()  // Start the tick system loop (uses tick_duration)

    initRedis()    // Initialize Redis
    startServer()  // Start the TCP server to accept client connections
}