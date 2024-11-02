import os
import socket
import time

from dotenv import load_dotenv

# Load environment variables from the .env file
load_dotenv()

# Get the Surge Protocol server host and port from environment variables
SERVER_HOST = os.getenv("SERVER_HOST", "localhost")  # Default to localhost if not set
SERVER_PORT = int(os.getenv("SERVER_PORT", 8080))     # Default to port 8080 if not set

# Function to continuously listen for messages
def listen_for_messages(sock):
    while True:
        try:
            data = sock.recv(1024).decode()
            if not data:
                print("Server closed the connection.")
                return
            print(f"Received from server: {data.strip()}")
        except socket.error as e:
            print(f"Connection error: {e}")
            return

# Function to connect to the server with retries on failure
def connect_to_server():
    while True:
        try:
            print(f"Attempting to connect to {SERVER_HOST}:{SERVER_PORT}...")
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.settimeout(5.0)
            sock.connect((SERVER_HOST, SERVER_PORT))
            print("Connected to the server.")
            return sock
        except socket.error as e:
            print(f"Connection failed: {e}. Retrying in 5 seconds...")
            time.sleep(5)

# Main function to maintain a perpetual connection
def main():
    while True:
        sock = connect_to_server()
        listen_for_messages(sock)
        sock.close()
        print("Connection lost. Reconnecting...")

if __name__ == "__main__":
    main()