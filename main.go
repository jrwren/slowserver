package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

func main() {
	var httpPort, httpsPort int
	var certfile string
	flag.IntVar(&httpPort, "httpPort", 8080, "http listen port")
	flag.IntVar(&httpsPort, "httpsPort", 8443, "https listen port")
	flag.StringVar(&certfile, "certfile", "", "certificate file for https")
	flag.Parse()

	r := http.NewServeMux()
	r.HandleFunc("/", slow)
	r.HandleFunc("/slow", slow)
	r.Handle("/ws-echo", websocket.Handler(echoServer))
	r.Handle("/ws-pinger", websocket.Handler(pinger))
	go func() {
		if certfile == "" {
			return
		}
		err := http.ListenAndServeTLS(":"+strconv.FormatInt(int64(httpPort), 10),
			certfile, certfile, r)
		log.Fatal(err)
	}()
	log.Fatal(http.ListenAndServe(":"+strconv.FormatInt(int64(httpPort), 10), r))
}

func slow(w http.ResponseWriter, r *http.Request) {
	// The slow return of this function is to take 5 minutes.
	// We shall return ~1MB total. and use american english dictionary for fun.
	f, err := os.Open("/usr/share/dict/words")
	if err != nil {
		log.Print("couldn't open /usr/share/dict/words")
		return
	}
	defer f.Close()
	r.ParseForm()
	help := `query params are chunk, delay, duration, help`
	if !strings.HasPrefix(r.Form.Get("help"), "n") {
		io.WriteString(w, help)
	}
	t := timeQueryParam(r.Form, "duration", 5*time.Minute)
	delay := timeQueryParam(r.Form, "delay", 2*time.Second)
	st, err := f.Stat()
	if err != nil {
		log.Print("couldn't stat /usr/share/dict/words")
		return
	}
	src, dst := f, w
	sz := int(st.Size())
	chunk := sz / int(t/delay)
	if c, err := strconv.ParseInt(r.Form.Get("chunk"), 10, 64); err == nil {
		chunk = int(c)
	} else {
		log.Print("failed to parse chunk query param",r.Form.Get("chunk"))
	}
	log.Printf("/slow writing %d every %s for %s", chunk, delay, t)
	w.Header().Set("content-length", strconv.Itoa(sz))
	buf := make([]byte, chunk)
	// lifted from io:
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errInvalidWrite
				}
			}
			w.(http.Flusher).Flush()
			time.Sleep(delay)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	if err != nil {
		log.Printf("/slow error writing %s", err)
	}
}

// Echo the data received on the WebSocket.
func echoServer(ws *websocket.Conn) {
	io.Copy(ws, ws)
}

func pinger(ws *websocket.Conn) {
	buf := make([]byte, 1500)
	n := 0
	for {
		ws.SetReadDeadline(time.Now().Add(1 * time.Second))
		br, err := ws.Read(buf)
		if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
			if errors.Is(err, io.EOF) {
				return
			}
			log.Printf("pinger read error: %s %T", err,err)
			return
		}
		if br>0 {
			log.Printf("pinger read: %s", buf[:br])
		}
		// This is probably terrible, but why not roll with it for now.
		time.Sleep(10 * time.Second)
		n++
		_, err = fmt.Fprintf(ws, "%d\n", n)
		if err != nil {
			log.Printf("pinger write error: %s", err)
			return
		}
	}
}

func timeQueryParam(v url.Values, name string, t time.Duration) time.Duration {
	d := v.Get(name)
	if d != `` {
		if t2, err := time.ParseDuration(d); err == nil {
			t = t2
		} else {
			log.Print("couldn't parse query parameter", name, d, err)
		}
	}
	return t
}

// errInvalidWrite means that a write returned an impossible count.
var errInvalidWrite = errors.New("invalid write result")
