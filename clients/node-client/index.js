const net = require('net');
const dotenv = require('dotenv');

// Load environment variables from .env file
dotenv.config();

const SERVER_HOST = process.env.SERVER_HOST || 'localhost';   // Default to 'localhost' if not set
const SERVER_PORT = process.env.SERVER_PORT || 8080;          // Default to port 8080
const TIMEOUT = Number(process.env.TIMEOUT) || 5000;          // Default 5 seconds timeout (5000ms)

// Function to listen for messages from the server
function listenForMessages(socket) {
    socket.on('data', function(data) {
        const message = data.toString().trim();
        console.log(`Received from server: ${message}`);
    });

    socket.on('timeout', function() {
        console.error('Connection timeout. Socket will be closed.');
        socket.end();  // Close the connection
        setTimeout(connectToServer, 5000);  // Retry connecting after 5 seconds
    });

    socket.on('error', function(err) {
        console.error(`Connection error: ${err.message}`);
        socket.destroy();  // Terminate the socket on error
    });

    socket.on('close', function() {
        console.log('Connection closed by the server.');
    });
}

// Function to connect to the server, retry on failure
function connectToServer() {
    console.log(`Attempting to connect to ${SERVER_HOST}:${SERVER_PORT}...`);

    const socket = new net.Socket();

    // Set connection timeout
    socket.setTimeout(TIMEOUT);

    socket.connect(SERVER_PORT, SERVER_HOST, function() {
        console.log('Connected to the server.');
        socket.setTimeout(0);  // Disable the timeout once connected
    });

    // Handle events (message listening, timeout, error, etc.)
    listenForMessages(socket);

    // Handle reconnection on socket close
    socket.on('close', function() {
        console.log('Connection lost. Reconnecting in 5 seconds...');
        setTimeout(connectToServer, 5000);  // Retry connecting after 5 seconds
    });
}

// Start client and connect to the server
connectToServer();