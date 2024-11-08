package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fogleman/gg"
	"github.com/go-redis/redis/v8"
	"golang.org/x/net/context"
)

const pngSquareSize = 15

var (
	rdb    *redis.Client
	ctx    = context.Background()
	mu     sync.Mutex
	conns  = make(map[net.Conn]struct{})
	config Config
	grid   [][]*GridCell // In-memory grid to store game state
)

// Draw the grid and export it as a PNG file
func drawGrid(filename string) error {
	width := config.GridWidth * pngSquareSize
	height := config.GridHeight * pngSquareSize

	dc := gg.NewContext(width, height)
	dc.SetRGB(1, 1, 1) // White background
	dc.Clear()

	// Draw each cell in the grid
	for x := 0; x < config.GridWidth; x++ {
		for y := 0; y < config.GridHeight; y++ {
			cell := grid[x][y]

			// Calculate the top-left corner of the square for this cell
			posX := x * pngSquareSize
			posY := y * pngSquareSize

			// Draw a square and symbol based on the entity type
			if cell.Spawn != nil {
				// Blue square with white "S"
				drawSquare(dc, posX, posY, "S", 0, 0, 1, 1, 1, 1)
			} else if cell.PowerNode != nil {
				// Green square with black "E"
				drawSquare(dc, posX, posY, "E", 0, 1, 0, 0, 0, 0)
			} else if cell.Spawn == nil && cell.PowerNode == nil && cell.PowerLink == nil && cell.Robot == nil {
				// Empty cell, display as gray
				drawSquare(dc, posX, posY, "", 0.7, 0.7, 0.7, 0, 0, 0)
			} else {
				//Cell with multiple components
				drawSquare(dc, posX, posY, "*", 0.0, 0.0, 0.0, 1, 1, 1)
			}
		}
	}

	var result = dc.SavePNG(filename)

	log.Printf("PNG Updated: %s", filename)

	// Save the image as a PNG
	return result
}

// Helper function to draw a square with a symbol at a specified position
func drawSquare(dc *gg.Context, x, y int, symbol string, r, g, b, textR, textG, textB float64) {
	// Draw the square fill
	dc.SetRGB(r, g, b) // Fill color
	dc.DrawRectangle(float64(x), float64(y), pngSquareSize, pngSquareSize)
	dc.Fill()

	// Draw the square outline (border)
	dc.SetRGB(0, 0, 0) // Black outline color
	dc.SetLineWidth(1) // Set the line width for the border
	dc.DrawRectangle(float64(x), float64(y), pngSquareSize, pngSquareSize)
	dc.Stroke()

	// Draw the symbol inside the square
	if symbol != "" {
		dc.SetRGB(textR, textG, textB) // Text color
		dc.DrawStringAnchored(symbol, float64(x)+pngSquareSize/2, float64(y)+pngSquareSize/2, 0.5, 0.5)
	}
}

// Config struct for reading JSON configuration
type Config struct {
	TickDuration     int    `json:"tick_duration"` // In seconds
	ServerPort       string `json:"server_port"`
	GridWidth        int    `json:"grid_width"`
	GridHeight       int    `json:"grid_height"`
	IsDevEnvironment bool   `json:"is_dev_environment"`
}

// Grid object types
type Spawn struct {
	CooldownUntil  int `json:"cooldown_until"`  // Tick when available again
	CooldownAmount int `json:"cooldown_amount"` // Number of ticks cooldown after use
	EnergyRequired int `json:"energy_required"` // Power required to spawn a robot
}

type PowerNode struct {
	EnergyProducedPerTick int `json:"energy_produced_per_tick"` // Energy produced each tick
}

type PowerLink struct {
	BuiltBy string `json:"built_by"` // Player who built the link
	Health  int    `json:"health"`   // Current health of the link
}

type Robot struct {
	Owner        string `json:"owner"`         // Player who owns the robot
	Health       int    `json:"health"`        // Health of the robot
	Energy       int    `json:"energy"`        // Energy of the robot
	QueuedAction string `json:"queued_action"` // Next action the robot will perform
}

type GridCell struct {
	Spawn     *Spawn     `json:"spawn,omitempty"`      // Spawn point for robots
	PowerNode *PowerNode `json:"power_node,omitempty"` // Node that produces energy
	PowerLink *PowerLink `json:"power_link,omitempty"` // Link that transmits power
	Robot     *Robot     `json:"robot,omitempty"`      // Robot controlled by the player
}

// Load configuration from the config.json file
func loadConfig() error {
	file, err := os.Open("config.json")
	if err != nil {
		return err
	}
	defer file.Close()

	byteValue, err := io.ReadAll(file)
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
	Tick    int               `json:"tick"`
	Players map[string]Player `json:"players"` // Map of apiKey -> Player
}

type Player struct {
	ApiKey   string   `json:"api_key"`
	Name     string   `json:"name"`
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

func createRobotForPlayer(apiKey string) error {
	// Collect all spawn points
	spawnLocations := make([][2]int, 0)
	for x := 0; x < config.GridWidth; x++ {
		for y := 0; y < config.GridHeight; y++ {
			if cell := grid[x][y]; cell != nil && cell.Spawn != nil {
				spawnLocations = append(spawnLocations, [2]int{x, y})
			}
		}
	}

	// Check if any spawn points are available
	if len(spawnLocations) == 0 {
		log.Println("No available spawn points found for player.")
		return fmt.Errorf("no available spawn points")
	}

	// Select a random spawn point from the available spawn points
	chosenSpawn := spawnLocations[rand.Intn(len(spawnLocations))]
	x, y := chosenSpawn[0], chosenSpawn[1]

	// Create the robot and assign it to the chosen spawn location
	newRobot := &Robot{
		Owner:        apiKey,
		Health:       100, // Default health
		Energy:       50,  // Default energy
		QueuedAction: "",  // No action queued initially
	}
	grid[x][y].Robot = newRobot

	// Save the updated grid cell to Redis
	key := fmt.Sprintf("grid:%d:%d", x, y)
	data := map[string]interface{}{
		"type":          "robot",
		"owner":         newRobot.Owner,
		"health":        newRobot.Health,
		"energy":        newRobot.Energy,
		"queued_action": newRobot.QueuedAction,
	}
	if err := rdb.HSet(ctx, key, data).Err(); err != nil {
		log.Printf("Failed to save robot at spawn location (%d, %d): %v", x, y, err)
		return err
	}

	log.Printf("Robot created for player %s at spawn point (%d, %d)", apiKey, x, y)
	return nil
}

// Parse commands from clients
func parseCommand(conn net.Conn, input string, state *GameState) {
	parts := strings.Split(strings.TrimSpace(input), " ")
	if len(parts) == 0 {
		conn.Write([]byte("ERROR: Invalid command format\n"))
		return
	}

	log.Printf("\n\nPARTS 0: %s\n\n", parts[0])

	helpString := `
# COMMANDS:

HELP
INIT_PLAYER <PLAYERNAME>

# QUEUEING COMMANDS FOR THIS TICK

COMMAND <APIKEY> <COMMANDNAME> <PARAMETER1> <PARAMETER2>

# SENDING YOUR COMMANDS FOR EXECUTION

COMMIT <APIKEY>`

	switch parts[0] {
	case "HELP":
		conn.Write([]byte(helpString))
		return

	case "INIT_PLAYER":
		apiKey := generateApiKey()

		if len(parts) < 2 {
			conn.Write([]byte("ERROR: Invalid INIT_PLAYER format: INIT_PLAYER NAME\n"))
			return
		}

		name := parts[1]

		if _, exists := state.Players[apiKey]; exists {
			conn.Write([]byte("ERROR: Player already exists\n"))
			return
		}

		// Create a new player
		newPlayer := Player{ApiKey: apiKey, Name: name, Commands: []string{}}
		state.Players[apiKey] = newPlayer

		// Create a robot at a random spawn location for the new player
		if err := createRobotForPlayer(apiKey); err != nil {
			conn.Write([]byte("ERROR: Could not create robot for player\nREPORT TO ADMINISTRATOR."))
			return
		}

		conn.Write([]byte("OK: Player initialized and robot created at a spawn point\n"))
		conn.Write([]byte(fmt.Sprintf("API_KEY FOR %s: %s\n", name, apiKey)))

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

func loadGridFromRedis() [][]*GridCell {
	loadedGrid := make([][]*GridCell, config.GridWidth)
	for x := 0; x < config.GridWidth; x++ {
		loadedGrid[x] = make([]*GridCell, config.GridHeight)
	}

	iter := rdb.Scan(ctx, 0, "grid:*", 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()

		var x, y int
		_, err := fmt.Sscanf(key, "grid:%d:%d", &x, &y)
		if err != nil {
			log.Printf("Failed to parse grid coordinates from key %s: %v", key, err)
			continue
		}

		cellData, err := rdb.HGetAll(ctx, key).Result()
		if err != nil {
			log.Printf("Failed to load cell data from Redis for %s: %v", key, err)
			continue
		}

		cell := &GridCell{}
		cellType := cellData["type"]

		switch cellType {
		case "spawn":
			cell.Spawn = &Spawn{
				CooldownUntil:  atoi(cellData["cooldown_until"]),
				CooldownAmount: atoi(cellData["cooldown_amount"]),
				EnergyRequired: atoi(cellData["energy_required"]),
			}
		case "power_node":
			cell.PowerNode = &PowerNode{
				EnergyProducedPerTick: atoi(cellData["energy_produced_per_tick"]),
			}
		case "power_link":
			cell.PowerLink = &PowerLink{
				BuiltBy: cellData["built_by"],
				Health:  atoi(cellData["health"]),
			}
		case "robot":
			cell.Robot = &Robot{
				Owner:        cellData["owner"],
				Health:       atoi(cellData["health"]),
				Energy:       atoi(cellData["energy"]),
				QueuedAction: cellData["queued_action"],
			}
		}

		loadedGrid[x][y] = cell
	}

	if err := iter.Err(); err != nil {
		log.Fatalf("Error iterating through Redis keys: %v", err)
	}

	log.Println("Game grid with entities successfully loaded from Redis.")
	return loadedGrid
}

// Helper function to convert string to int
func atoi(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

func initializeGameGrid() {
	// Check if the grid has already been initialized in Redis
	exists, err := rdb.Exists(ctx, "grid-initialized").Result()
	if err != nil {
		log.Fatalf("Error checking grid initialization in Redis: %v", err)
	}

	if exists > 0 {
		// Grid exists in Redis; load it into memory
		log.Println("Loading existing game grid from Redis.")
		grid = loadGridFromRedis()
	} else {
		// Grid does not exist; initialize a new one in memory and save it
		log.Println("No grid found in Redis; initializing new game grid.")
		initializeInMemoryGrid()
		saveGridToRedis()

		// Mark grid as initialized in Redis
		if err := rdb.Set(ctx, "grid-initialized", 1, 0).Err(); err != nil {
			log.Fatalf("Failed to mark grid as initialized in Redis: %v", err)
		}
	}
}

func initializeInMemoryGrid() {
	grid = make([][]*GridCell, config.GridWidth)
	for x := 0; x < config.GridWidth; x++ {
		grid[x] = make([]*GridCell, config.GridHeight)
		for y := 0; y < config.GridHeight; y++ {
			cell := &GridCell{}

			randVal := rand.Float64()
			switch {
			case randVal < (0.001): // 5% chance for a Spawn object
				cell.Spawn = &Spawn{
					CooldownUntil:  0,
					CooldownAmount: 10, // Example cooldown value
					EnergyRequired: 50, // Example energy required
				}
			case randVal < (0.025): // Additional 10% for PowerNode
				cell.PowerNode = &PowerNode{
					EnergyProducedPerTick: 10, // Example energy produced
				}
			}

			grid[x][y] = cell
		}
	}
	log.Println("In-memory game grid initialized with various entity types.")
}

func saveGridToRedis() {
	for x := 0; x < config.GridWidth; x++ {
		for y := 0; y < config.GridHeight; y++ {
			cell := grid[x][y]
			if cell == nil {
				continue
			}

			key := fmt.Sprintf("grid:%d:%d", x, y)
			data := make(map[string]interface{})

			if cell.Spawn != nil {
				data["type"] = "spawn"
				data["cooldown_until"] = cell.Spawn.CooldownUntil
				data["cooldown_amount"] = cell.Spawn.CooldownAmount
				data["energy_required"] = cell.Spawn.EnergyRequired
			} else if cell.PowerNode != nil {
				data["type"] = "power_node"
				data["energy_produced_per_tick"] = cell.PowerNode.EnergyProducedPerTick
			} else if cell.PowerLink != nil {
				data["type"] = "power_link"
				data["built_by"] = cell.PowerLink.BuiltBy
				data["health"] = cell.PowerLink.Health
			} else if cell.Robot != nil {
				data["type"] = "robot"
				data["owner"] = cell.Robot.Owner
				data["health"] = cell.Robot.Health
				data["energy"] = cell.Robot.Energy
				data["queued_action"] = cell.Robot.QueuedAction
			}

			if len(data) > 0 {
				err := rdb.HSet(ctx, key, data).Err()
				if err != nil {
					log.Printf("Failed to save cell at (%d, %d): %v", x, y, err)
				}
			}
		}
	}
	log.Println("In-memory game grid with entities saved to Redis.")
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
		saveGridToRedis()
		// Export the game state to JSON
		if err := exportGameStateToJSON("/app/shared/game_state.json", state); err != nil {
			log.Fatalf("Failed to export game state to JSON: %v", err)
		}

		// Draw the grid to a PNG file
		if err := drawGrid("/app/shared/grid_output.png"); err != nil {
			log.Fatalf("Failed to draw grid: %v", err)
		}
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

func nukeEverything() {
	// Create a context
	ctx := context.Background()
	// Execute FLUSHDB command
	err := rdb.FlushDB(ctx).Err()
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Database flushed")
	}
}

// Export the entire game state and grid to a JSON file
func exportGameStateToJSON(filename string, state *GameState) error {
	// Create a structure to hold the entire game state for export
	exportData := struct {
		Tick    int               `json:"tick"`
		Players map[string]Player `json:"players"`
		Grid    [][]*GridCell     `json:"grid"`
	}{
		Tick:    state.Tick,
		Players: state.Players,
		Grid:    grid,
	}

	// Marshal the export data to JSON
	jsonData, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal game state to JSON: %v", err)
		return err
	}

	// Write JSON to the specified file
	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		log.Printf("Failed to write game state to file: %v", err)
		return err
	}

	log.Printf("Game state successfully exported to %s", filename)
	return nil
}

// Serve the exported JSON file over HTTP on port 80
func serveJSONFile(filename string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filename)
	})

	log.Println("Serving JSON file on port 80...")
	if err := http.ListenAndServe(":80", nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}

func main() {
	if err := loadConfig(); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	initRedis() // Initialize Redis connection

	if config.IsDevEnvironment {
		log.Println("Dev Environment Detected...")
		nukeEverything()
	} else {
		log.Println("Production Environment Detected...")
	}

	state := loadOrInitGameState() // Load or initialize game state

	initializeGameGrid()

	go gameLoop(state) // Start the tick system loop

	// Serve the game state JSON file over HTTP on port 80
	go serveJSONFile("/app/shared/game_state.json")

	startServer(state) // Start the TCP server to accept client connections
}
