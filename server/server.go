package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
)

// a channel where the server can send messages to the client
// chan string	| bidirectional
// chan<- string | only send
// <-chan string | only receive
type client chan<- string

// define a dataclass for struct a message
type message struct {
	text   string
	sender client
}

var (
	entering = make(chan client)
	leaving  = make(chan client)
	messages = make(chan message)
)

func main() {
	listener, err := net.Listen("tcp", "localhost:8000")
	if err != nil {
		log.Fatal(err)
	}

	// defer the closing of the listener until the main function returns
	// (defer will run this when the host function returns, which is "main" in this case)
	defer listener.Close()

	fmt.Println("Chat server listening on localhost:8000")

	// start the broadcaster goroutine (threads) that will handle broadcasting messages to clients
	go broadcaster()

	for {

		// blocks until a new client connects to the server, then returns a new connection object for that client
		conn, err := listener.Accept()
		if err != nil {
			log.Print(err)
			continue
		}
		fmt.Println(conn.RemoteAddr().String() + " has join")

		// start a new goroutine to handle the connection for each client
		go handleConn(conn)
	}
}

func broadcaster() {
	clients := make(map[client]bool)

	// always listening for messages
	for {

		// switch case
		select {

		// when a message is received on the messages channel
		// broadcast it to all clients
		case msg := <-messages:
			for cli := range clients {
				if cli != msg.sender {
					cli <- msg.text // the <- operator is used to send the message to the client channel
				}
			}

		// when a new client enters, add it to the clients map
		case cli := <-entering:
			clients[cli] = true

		// when a client leaves, remove it from the clients map and close the channel
		case cli := <-leaving:
			delete(clients, cli)
			close(cli)
		}
	}
}

func handleConn(conn net.Conn) {
	ch := make(chan string) // a channel to send messages to this client

	// start a goroutine to write messages from the channel to the client connection
	go clientWriter(conn, ch)

	who := conn.RemoteAddr().String() // get the client's remote addr as string
	ch <- ""                          // send an empty line
	ch <- "You are" + who             // send a message to the channel (client only)

	messages <- message{text: who + " has join", sender: ch} // send a message to all channels
	entering <- ch                                           // register the new client channel in the broadcaster

	// read the inputs from the server console
	input := bufio.NewScanner(conn)
	for input.Scan() {
		messages <- message{text: who + ": " + input.Text(), sender: ch}
	}
	// note: the for is blocked until the conn is exited
	clientLeaving(conn, ch, who)
}

func clientWriter(conn net.Conn, ch <-chan string) {
	// in the client's socket, write all incoming messages
	for msg := range ch {
		fmt.Fprintln(conn, msg)
	}
}

func clientLeaving(conn net.Conn, ch client, who string) {
	leaving <- ch
	messages <- message{text: who + " has left", sender: ch}
	fmt.Println(conn.RemoteAddr().String() + " has left")
	conn.Close()
}
