package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
)

func main() {
	nameFlag := flag.String("name", "", "Your name in the chat")
	roomFlag := flag.String("room", "", "The room to join")

	flag.Parse()

	name := strings.Trim(*nameFlag, " ")
	room := strings.Trim(*roomFlag, " ")

	if name == "" {
		panic("You must provide a name using the --name flag")
	}

	conn, err := net.Dial("tcp", "localhost:8000")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	fmt.Fprintf(conn, "NAME %s\n", name)

	if room != "" {
		fmt.Fprintf(conn, "JOIN %s\n", room)
	}

	// anon goroutine to read from the conn and print it
	go func() {
		io.Copy(os.Stdout, conn)
		log.Println("Connection closed by the server")
		os.Exit(0)
	}() // run it right away

	// main thread to read from the keyboard and send it to the channel
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Fprintln(conn, scanner.Text())
	}
}
