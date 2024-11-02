package main

import (
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net"
    "os"
    "sync"
    "time"
    "strings"
    
    "github.com/go-redis/redis/v8"
    "golang.org/x/net/context"
)

var (
    rdb     *redis.Client
    ctx     = context.Background()
    mu      sync.Mutex
    conns   = make(map[net.Conn]struct{})
    config  Config
)

// Config struct for reading JSON configuration
type Config struct {
    TickDuration int    `json:"tick_duration"`
    ServerPort   string `json:"server_port"`
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

// GameState struct, stored in Redis
type GameState struct {
    Tick       int            `json:"tick"`
    Players    map[string]Player `json:"players"` // Map of apiKey -> Player
}

type Player struct {
    ApiKey  string `json:"api_key"`
    Name    string `json:"name"`
    Commands []string `json:"commands"` // Buffered commands
}

// Load or Initialize Game State from Redis
func loadOrInitGameState() *GameState {
    state := &GameState{}
    result, err := rdb.Get(ctx, "game:state").Result()
    if err == redis.Nil {
        // If no game state is found, initialize
        state = &GameState{
            Tick:    0,
            Players: make(map[string]Player),
        }
        saveGameState(*state)
        log.Println("Initialized new game state.")
    } else if err != nil {
        log.Fatalf("Failed to load game state from Redis: %v", err)
    } else {
        if err := json.Unmarshal([]byte(result), state); err != nil {
            log.Fatalf("Failed to parse game state: %v", err)
        }
        log.Println("Loaded game state from Redis.")
    }
    return state
}

// Save game state in Redis
func saveGameState(state GameState) {
    data, _ := json.Marshal(state)
    if err := rdb.Set(ctx, "game:state", data, 0).Err(); err != nil {
        log.Fatalf("Failed to store game state: %v", err)
    }
}

// Generate a new API key for a player
func generateApiKey() string {
    key := make([]byte, 16)
    _, err := rand.Read(key)
    if err != nil {
        log.Fatalf("Error generating API key: %v", err)
    }
    return hex.EncodeToString(key)
}

// Parse commands from clients
func parseCommand(conn net.Conn, input string, state *GameState) {
    parts := strings.Split(strings.TrimSpace(input), " ")
    if len(parts) == 0 {
        conn.Write([]byte("ERROR: Invalid command format\n"))
        return
    }

    log.Printf("\n\nPARTS 0: %s\n\n", parts[0])

    switch parts[0] {
    case "NEW_API_KEY":
        apiKey := generateApiKey()
        conn.Write([]byte(fmt.Sprintf("NEW_API_KEY %s\n", apiKey)))
        return

    case "INIT_PLAYER":
        if len(parts) < 3 {
            conn.Write([]byte("ERROR: Invalid INIT_PLAYER format: INIT_PLAYER API_KEY NAME\n"))
            return
        }
        apiKey := parts[1]
        name := parts[2]
        if _, exists := state.Players[apiKey]; exists {
            conn.Write([]byte("ERROR: Player already exists\n"))
            return
        }
        state.Players[apiKey] = Player{ApiKey: apiKey, Name: name, Commands: []string{}}
        saveGameState(*state)
        conn.Write([]byte("OK: Player initialized\n"))

    case "COMMAND":
        if len(parts) < 3 {
            conn.Write([]byte("ERROR: COMMAND requires API key and action\n"))
            return
        }
        apiKey := parts[1]
        if player, exists := state.Players[apiKey]; !exists {
            conn.Write([]byte("ERROR: Player not found\n"))
        } else {
            action := parts[2:] // Store the rest as a command
            commandStr := formatCommand(action)
            player.Commands = append(player.Commands, commandStr)
            state.Players[apiKey] = player
            saveGameState(*state)
            conn.Write([]byte("OK: Command staged\n"))
        }

    case "COMMIT":
        if len(parts) < 2 {
            conn.Write([]byte("ERROR: COMMIT requires API key\n"))
            return
        }
        apiKey := parts[1]
        if player, exists := state.Players[apiKey]; !exists {
            conn.Write([]byte("ERROR: Player not found\n"))
        } else {
            // Execute commands
            executeCommands(player.Commands)
            player.Commands = []string{} // Clear the command queue once executed
            state.Players[apiKey] = player
            saveGameState(*state)
            conn.Write([]byte("OK: Commands committed\n"))
        }

    default:
        conn.Write([]byte(fmt.Sprintf("ERROR: Unknown command %s\n", parts[0])))
    }
}

func formatCommand(parts []string) string {
    return fmt.Sprintf("%s", parts)
}

func executeCommands(commands []string) {
    for _, cmd := range commands {
        log.Printf("Executing command: %s", cmd)
        // Actual game logic to execute command goes here
    }
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
func gameLoop(state *GameState) {
    for {
        time.Sleep(time.Duration(config.TickDuration) * time.Second)
        state.Tick++
        log.Printf("Tick %d", state.Tick)
        sendTickMessage(state.Tick)

        // Store the tick count in Redis
        saveGameState(*state)
    }
}

// Handle incoming client connections
func handleConnection(conn net.Conn, state *GameState) {
    log.Printf("New client connected: %v", conn.RemoteAddr())

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

    for {
        buf := make([]byte, 1024)
        length, err := conn.Read(buf)
        if err != nil {
            return
        }
        input := string(buf[:length])
        log.Printf("Received: %s", input)
        parseCommand(conn, input, state)
    }
}

// Start the TCP server that listens for client connections
func startServer(state *GameState) {
    address := fmt.Sprintf(":%s", config.ServerPort)
    listener, err := net.Listen("tcp", address)
    if err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }
    log.Printf("Server listening on port %s", config.ServerPort)

    for {
        conn, err := listener.Accept()
        if err != nil {
            log.Println("Error accepting connection:", err)
            continue
        }
        go handleConnection(conn, state)
    }
}

// Initialize Redis connection
func initRedis() {
    rdb = redis.NewClient(&redis.Options{
        Addr: "redis:6379",
    })
    if _, err := rdb.Ping(ctx).Result(); err != nil {
        log.Fatalf("Failed to connect to Redis: %v", err)
    }
    log.Println("Connected to Redis.")
}

func main() {
    if err := loadConfig(); err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }

    initRedis()    // Initialize Redis connection
    state := loadOrInitGameState() // Load or initialize game state

    go gameLoop(state)  // Start the tick system loop

    startServer(state)  // Start the TCP server to accept client connections
}