# Surge Protocol Game

## Premise

Surge Protocol immerses players in a world where a rogue AI, known only as **The Machine**, seeks to disrupt Earth's energy network. Players, taking control of specialized robots, must restore control over the grid by managing nodes, repairing links, and fending off attacks. As the AI escalates, so too must the players' strategy in preventing total energy collapse. Will you save the grid or burn out amidst the Machine's relentless surge?

## Game Objectives

- **Establish Control**: Claim and secure energy nodes in the grid.
- **Defend**: Battle and defend against disruptions from **The Machine**.
- **Expand Your Network**: Grow your claimed area by linking nodes.
- **Energy Management**: Use energy wisely to perform essential robot tasks like moving, harvesting, and repairing.
- **Cooperate**: Work together with other players to fend off escalating attacks.

## Getting Started

### Prerequisites

You will need:
- A TCP Client
- Your programming language of choice (Python, Node.js, Go)

### Game Protocol

#### Connecting to the server

To start playing, you need to connect to the game's TCP server. The IP address and port will be provided by the game host. Once connected, you can issue various commands to interact with the game world.

#### Commands

Each action performed in the game is done via commands. **All commands require an API key** that represents your player.

- **NEW_API_KEY**: Generate a new API key.
    ```plaintext
    NEW_API_KEY
    ```
    Example response:
    ```plaintext
    NEW_API_KEY 7bb113b3a9834b7a8fc
    ```

- **INIT_PLAYER**: Register a new player using an API key and a name.
    ```plaintext
    INIT_PLAYER <api_key> <player_name>
    ```
    Example:
    ```plaintext
    INIT_PLAYER 7bb113b3a9834b7a8fc PlayerOne
    ```

- **COMMAND**: Queue a command for your player using their API key. Example actions could be `MOVE`, `HARVEST`, or `REPAIR`.
    ```plaintext
    COMMAND <api_key> <command> <parameters>
    ```
    Example:
    ```plaintext
    COMMAND 7bb113b3a9834b7a8fc MOVE 12 15
    ```

- **COMMIT**: After queueing actions, commit them to be executed in the next tick.
    ```plaintext
    COMMIT <api_key>
    ```
    Example:
    ```plaintext
    COMMIT 7bb113b3a9834b7a8fc
    ```

Responses will either be:
- `OK` when the command was successful.
- `ERROR` when an issue occurred.

#### Game Flow

- The game runs in **ticks** (a regular interval defined by the server).
- Players must **queue commands** relevant to their robots and then send a **COMMIT** to confirm the execution of these commands.
- The game's state updates at each tick, and your actions take effect after the next tick.

## Sample Code

Below are examples of how to interact with Surge Protocol's server in various programming languages.

### Python

```python
import socket

# Connect to server
sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
sock.connect(('localhost', 8080))

# Generate new API key
sock.sendall(b'NEW_API_KEY\n')
response = sock.recv(1024)
print(response.decode('utf-8'))

# Assuming the API key is returned as 'NEW_API_KEY 7bb113b3a9834b7a8fc'
api_key = '7bb113b3a9834b7a8fc'

# Initialize player
sock.sendall(f'INIT_PLAYER {api_key} PythonPlayer\n'.encode())
response = sock.recv(1024)
print(response.decode('utf-8'))

# Queue a move command and commit
sock.sendall(f'COMMAND {api_key} MOVE 12 15\n'.encode())
response = sock.recv(1024)
print(response.decode('utf-8'))

sock.sendall(f'COMMIT {api_key}\n'.encode())
response = sock.recv(1024)
print(response.decode('utf-8'))
```

### Node

```javascript
const net = require('net');

// Connect to server
const client = new net.Socket();
client.connect(8080, 'localhost', () => {
    console.log('Connected to Surge Protocol server');

    // Generate a new API key
    client.write('NEW_API_KEY\n');
});

client.on('data', (data) => {
    console.log('Received: ' + data);

    // Assuming the API key is returned as 'NEW_API_KEY 7bb113b3a9834b7a8fc'
    const apiKey = '7bb113b3a9834b7a8fc';

    // Initialize player
    if (data.toString().includes('NEW_API_KEY')) {
        client.write(`INIT_PLAYER ${apiKey} NodePlayer\n`);
    } else if (data.toString().includes('OK')) {
        // Queue a move command and commit
        client.write(`COMMAND ${apiKey} MOVE 12 15\n`);
        client.write(`COMMIT ${apiKey}\n`);
    }
});
```

### Go

```go
package main

import (
    "bufio"
    "fmt"
    "net"
    "strings"
)

func main() {
    // Connect to the server
    conn, _ := net.Dial("tcp", "localhost:8080")

    // Create a new API key
    fmt.Fprintf(conn, "NEW_API_KEY\n") 
    reply, _ := bufio.NewReader(conn).ReadString('\n')
    fmt.Println("Response:", reply)

    // Assuming the API key is returned as 'NEW_API_KEY 7bb113b3a9834b7a8fc'
    apiKey := strings.TrimSpace(strings.Split(reply, " ")[1])

    // Initialize player
    fmt.Fprintf(conn, "INIT_PLAYER %s GoPlayer\n", apiKey)
    reply, _ = bufio.NewReader(conn).ReadString('\n')
    fmt.Println("Response:", reply)

    // Queue a move command
    fmt.Fprintf(conn, "COMMAND %s MOVE 12 15\n", apiKey)
    reply, _ = bufio.NewReader(conn).ReadString('\n')
    fmt.Println("Response:", reply)

    // Commit the actions
    fmt.Fprintf(conn, "COMMIT %s\n", apiKey)
    reply, _ = bufio.NewReader(conn).ReadString('\n')
    fmt.Println("Response:", reply)
}
```
