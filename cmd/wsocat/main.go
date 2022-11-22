package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	xnws "golang.org/x/net/websocket"

	"github.com/jrwren/slowserver/internal/res"
)

func main() {
	var resolve string
	var k, b, useXNWS bool
	flag.StringVar(&resolve, "resolve", "", "<host:port:addr[,addr]...> Use custom addr to override DNS.")
	flag.BoolVar(&useXNWS, "useXNWS", false, "Use golang.org/x/net/websocket for raw socket instead of data framing as defined in RFC6455.")
	flag.BoolVar(&k, "k", false, "Allow insecure connections when using TLS.")
	flag.BoolVar(&b, "b", false, "Use binary messages.")
	flag.Parse()
	url := flag.Arg(0)

	if useXNWS && resolve != "" {
		fmt.Printf("resolve and useXNWS are mutually exclusive options")
		flag.PrintDefaults()
		return
	}

	if useXNWS {
		origin := strings.Replace(url, "ws:", "http:", 1)
		origin = strings.Replace(origin, "wss:", "https:", 1)

		ws, err := xnws.Dial(url, "", origin)
		if err != nil {
			log.Fatal(err)
		}
		go io.Copy(ws, os.Stdin)
		io.Copy(os.Stdout, ws)
		return
	}
	dila := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 5 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: k,
		},
	}

	if resolve != "" {
		r := strings.Split(resolve, ":")
		host := r[0]
		// port := r[1]
		addrs := r[2:]
		ao := &res.Override{H: host, Addrs: addrs}
		dila.NetDialContext = ao.Dial
	}
	ws, resp, err := dila.Dial(url, nil)
	if err != nil {
		log.Println("fatal error dialing websocket ", err)
		if err == websocket.ErrBadHandshake {
			log.Printf("%v %v %v\n", resp.StatusCode, resp.Status, resp.Header)
			io.Copy(os.Stderr, resp.Body)
		}
		return
	}
	mt := websocket.TextMessage
	if b {
		mt = websocket.BinaryMessage
	}
	go func() {
		w, err := ws.NextWriter(mt)
		if err != nil {
			log.Fatal("fatal error getting writer:", err)
		}
		io.Copy(w, os.Stdin)
	}()
	for {
		_, r, err:=ws.NextReader()
		if err != nil {
			log.Fatal("fatal error getting reader:", err)
		}
		io.Copy(os.Stdout, r)

	}
}
