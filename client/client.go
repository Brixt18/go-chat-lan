package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:8000")

	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

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
