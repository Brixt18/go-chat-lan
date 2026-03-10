package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// Basic types
type clientChan chan<- string

// Client state
type client struct {
	channel    clientChan
	connection net.Conn
	name       string
	room       string
}

// Commands sent to the broadcaster
type command struct {
	cmdType string  // "JOIN", "LEAVE", "MSG", "NAME"
	client  *client // Use a pointer so every goroutine sees the same client instance.
	arg     string  // Command argument, such as a room name or message.
}

var (
	// A single command channel centralizes all state changes.
	commands = make(chan command)
)

func main() {
	listener, err := net.Listen("tcp", "localhost:8000")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	fmt.Println("Chat server listening on localhost:8000")

	// The broadcaster is the single owner of shared state.
	go broadcaster()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Print(err)
			continue
		}
		fmt.Println(conn.RemoteAddr().String() + " has connected")
		go handleConn(conn)
	}
}

func broadcaster() {
	// All shared state lives here, so no other goroutine mutates it directly.
	// Room map: room name -> set of clients in that room.
	rooms := make(map[string]map[*client]bool)
	// Global set of every connected client.
	allClients := make(map[*client]bool)

	for cmd := range commands {
		switch cmd.cmdType {

		case "CONNECT":
			allClients[cmd.client] = true
			cmd.client.channel <- "Welcome! Set your name with 'SET NAME <name>'"

		case "DISCONNECT":
			delete(allClients, cmd.client)
			if cmd.client.room != "" {
				delete(rooms[cmd.client.room], cmd.client)
				broadcastToRoom(rooms[cmd.client.room], fmt.Sprintf("%s has left the room.", cmd.client.name), nil)
			}
			cmd.client.connection.Close()
			fmt.Println("Client disconnected:", cmd.client.connection.RemoteAddr().String())

		case "NAME":
			cmd.client.name = cmd.arg + "#" + cmd.client.connection.RemoteAddr().String()[5:]
			cmd.client.channel <- "Your name is now: " + cmd.client.name

		case "JOIN":
			// If the client was already in a room, remove them first.
			if cmd.client.room != "" {
				delete(rooms[cmd.client.room], cmd.client)
				broadcastToRoom(rooms[cmd.client.room], fmt.Sprintf("%s left the room.", cmd.client.name), nil)
			}

			// The room must already exist before the client can join it.
			if rooms[cmd.arg] == nil {
				cmd.client.channel <- fmt.Sprintf("The room %s does not exist.", cmd.arg)
				continue
			}

			// Add the client to the requested room.
			rooms[cmd.arg][cmd.client] = true
			cmd.client.room = cmd.arg

			cmd.client.channel <- "Joined room: " + cmd.arg
			broadcastToRoom(rooms[cmd.arg], fmt.Sprintf("%s joined the room.", cmd.client.name), cmd.client)

		case "CREATE":
			if rooms[cmd.arg] != nil {
				cmd.client.channel <- fmt.Sprintf("The room %s already exists.", cmd.arg)
				continue
			}
			rooms[cmd.arg] = make(map[*client]bool)
			cmd.client.channel <- fmt.Sprintf("Room %s created. You can now JOIN it.", cmd.arg)

		case "LEAVE":
			if cmd.client.room != "" {
				delete(rooms[cmd.client.room], cmd.client)
				broadcastToRoom(rooms[cmd.client.room], fmt.Sprintf("%s left the room.", cmd.client.name), nil)
				cmd.client.room = ""
				cmd.client.channel <- "You left the room."
			}

		case "LIST":
			if len(rooms) == 0 {
				cmd.client.channel <- "No rooms available."
			} else {
				cmd.client.channel <- "Rooms available:"
				for roomName, users := range rooms {
					cmd.client.channel <- fmt.Sprintf("- %s (Users: %d)", roomName, len(users))
				}
			}

		case "MSG":
			if cmd.client.room == "" {
				cmd.client.channel <- "You are not in a room. Use JOIN <room>"
			} else {
				// Format the message and broadcast it to the current room.
				fullMsg := fmt.Sprintf("[%s] %s>> %s", cmd.client.room, cmd.client.name, cmd.arg)
				broadcastToRoom(rooms[cmd.client.room], fullMsg, cmd.client)
			}

		case "ALL":
			if cmd.client.name == "Server" {
				broadcastToAll(allClients, cmd.arg)
			}

		case "WHISPER":
			parts := strings.SplitN(cmd.arg, " ", 2)
			if len(parts) < 2 {
				cmd.client.channel <- "Usage: WHISPER <user> <message>"
				continue
			}
			targetName := parts[0]
			message := parts[1]
			var targetClient *client
			for cli := range allClients {
				if cli.name == targetName {
					targetClient = cli
					break
				}
			}
			if targetClient == nil {
				cmd.client.channel <- fmt.Sprintf("User %s not found.", targetName)
			} else {
				targetClient.channel <- fmt.Sprintf("[WHISPER] %s>> %s", cmd.client.name, message)
			}

		case "USERS":
			if cmd.client.room == "" {
				cmd.client.channel <- "You are not in a room. Use JOIN <room>"
			} else {
				roomClients := rooms[cmd.client.room]
				cmd.client.channel <- fmt.Sprintf("Users in room %s:", cmd.client.room)
				for cli := range roomClients {
					if cli != cmd.client {
						cmd.client.channel <- fmt.Sprintf("- %s", cli.name)
					}
				}
			}
		}

	}
}

// Helper that broadcasts to a room. 'skip' avoids echoing the message back to the sender.
func broadcastToRoom(roomClients map[*client]bool, text string, skip *client) {
	for cli := range roomClients {
		if cli != skip {
			// Use a non-blocking send so one slow client does not stall the broadcaster.
			// This prevents slow clients from blocking room broadcasts.
			select {
			case cli.channel <- text:
			default:
				// The client is backed up; we could disconnect them here if needed.
			}
		}
	}
}

func broadcastToAll(allClients map[*client]bool, text string) {
	fullMsg := fmt.Sprintf("[SERVER ANNOUNCEMENT] %s", text)
	for cli := range allClients {
		select {
		case cli.channel <- fullMsg:
		default:
		}
	}
}

func handleConn(conn net.Conn) {
	ch := make(chan string)

	// Activity notifications are sent through this channel.
	// A buffer of 1 keeps activity updates from blocking the read loop.
	activityChan := make(chan bool, 1)

	// Keep a pointer so shared updates affect the same client value everywhere.
	c := &client{
		channel:    ch,
		connection: conn,
		name:       "Anonymous",
	}

	go clientWriter(conn, ch)
	go disconnectUser(c, activityChan)

	commands <- command{cmdType: "CONNECT", client: c}

	input := bufio.NewScanner(conn)
	for input.Scan() {
		select {
		case activityChan <- true:
		default:
		}

		text := input.Text()
		text = strings.TrimSpace(text)

		switch {
		case strings.HasPrefix(text, "NAME"):
			name := strings.TrimPrefix(text, "NAME ")
			name = strings.TrimSpace(name)
			if name == "" {
				c.channel <- "Name cannot be empty."
				continue
			} else if strings.Contains(name, " ") {
				c.channel <- "Name cannot contain spaces."
				continue
			}

			commands <- command{cmdType: "NAME", client: c, arg: name}

		case strings.HasPrefix(text, "JOIN"):
			commands <- command{cmdType: "JOIN", client: c, arg: strings.TrimPrefix(text, "JOIN ")}

		case strings.HasPrefix(text, "CREATE"):
			roomName := strings.TrimPrefix(text, "CREATE ")
			roomName = strings.TrimSpace(roomName)
			if roomName == "" {
				c.channel <- "Room name cannot be empty."
				continue
			} else if strings.Contains(roomName, " ") {
				c.channel <- "Room name cannot contain spaces."
				continue
			}

			commands <- command{cmdType: "CREATE", client: c, arg: roomName}

		case strings.HasPrefix(text, "WHISPER"):
			commands <- command{cmdType: "WHISPER", client: c, arg: strings.TrimPrefix(text, "WHISPER ")}

		case text == "LEAVE":
			commands <- command{cmdType: "LEAVE", client: c}

		case text == "LIST":
			commands <- command{cmdType: "LIST", client: c}

		case text == "USERS":
			commands <- command{cmdType: "USERS", client: c}

		default:
			// Anything else is treated as a regular chat message.
			commands <- command{cmdType: "MSG", client: c, arg: text}
		}
	}

	// If the read loop ends, the client has disconnected.
	commands <- command{cmdType: "DISCONNECT", client: c}
}

func clientWriter(conn net.Conn, ch <-chan string) {
	for msg := range ch {
		fmt.Fprintln(conn, msg)
	}
}

func disconnectUser(c *client, activityChan <-chan bool) {
	timeoutDuration := 10 * time.Second
	disonnectDuration := 10 * time.Second
	timer := time.NewTimer(timeoutDuration)
	disconnect := time.NewTimer(disonnectDuration)
	disconnect.Stop()

	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			c.channel <- "Due to inactivity, you will be disconnceted in 10 seconds. Send any message to stay connected."
			disconnect.Reset(disonnectDuration)

		case <-disconnect.C:
			fmt.Println("Desconectando a", c.connection.RemoteAddr().String(), "por inactividad.")
			c.channel <- "You have been disconnected due to inactivity."
			c.connection.Close()
			return

		case <-activityChan:
			fmt.Println("Timer reset for client " + c.connection.RemoteAddr().String())
			// Stop the timer safely using the standard Go pattern.
			if !timer.Stop() {
				// If the timer already fired but this case won the select,
				// drain the channel so it does not trigger on the next loop.
				select {
				case <-timer.C:
				default:
				}
			}
			if !disconnect.Stop() {
				select {
				case <-disconnect.C:
				default:
				}
			}
			// Start the inactivity countdown again.
			timer.Reset(timeoutDuration)
			disconnect.Stop()
		}
	}
}
