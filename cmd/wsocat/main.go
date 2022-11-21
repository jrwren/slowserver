package main

import (
	"flag"
	"io"
	"log"
	"os"
	"strings"

	"golang.org/x/net/websocket"
)

func main() {
	// TODO: Support disabling cert validation with -k.
	flag.Parse()
	url := flag.Arg(0)
	origin := strings.Replace(url, "ws:", "http:", 1)
	origin = strings.Replace(origin, "wss:", "https:", 1)
	ws, err := websocket.Dial(url, "", origin)
	if err != nil {
		log.Fatal(err)
	}
	go io.Copy(ws, os.Stdin)
	io.Copy(os.Stdout, ws)
}
