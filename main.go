// Copyright 2022 Cisco Inc. All Rights Reserved.

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/gorilla/websocket"
	xnws "golang.org/x/net/websocket"
)

var (
	conns []*Connection
)

func main() {
	err := os.WriteFile("/run/app.pid", []byte(strconv.Itoa(os.Getpid())), os.ModePerm)
	if err != nil {
		log.Println("could not write /run/app.pid")
	}
	var httpPort, httpsPort int
	var certfile, initconns string
	var gosocket bool
	flag.BoolVar(&gosocket, "gosocket", false, "serve websocket with golang.org/x/net/websocket insetad of ")
	flag.IntVar(&httpPort, "httpPort", 8080, "http listen port")
	flag.IntVar(&httpsPort, "httpsPort", 8443, "https listen port")
	flag.StringVar(&certfile, "certfile", "", "certificate file for https")
	// e.g. -initconns tcpbin.com:4242_35s_"ping\n"
	// or www.example.net:80_25s_"GET / HTTP/1.1\r\nHost: %s\r\n\r\n"
	flag.StringVar(&initconns, "initconns", "", "initial remote connections - comma separated host:port_delay_payload pairs")
	flag.Parse()
	doinitconns(initconns)
	log.Print("initialized ", len(conns), " connections")
	r := http.NewServeMux()
	r.Handle("/", gziphandler.GzipHandler(http.HandlerFunc(root)))
	r.Handle("/slow", gziphandler.GzipHandler(http.HandlerFunc(slow)))
	r.Handle("/slam", gziphandler.GzipHandler(http.HandlerFunc(slam)))
	r.Handle("/slam/headers", gziphandler.GzipHandler(http.HandlerFunc(headerSlam)))
	r.Handle("/slam/body", gziphandler.GzipHandler(http.HandlerFunc(bodySlam)))
	r.Handle("/connections", gziphandler.GzipHandler(http.HandlerFunc(connections)))
	r.Handle("/headers", gziphandler.GzipHandler(http.HandlerFunc(headers)))
	r.Handle("/gs-echo", xnws.Handler(echoServerXNWS))
	r.Handle("/gs-pinger", xnws.Handler(pingerXNWS))
	r.Handle("/ws-echo", gziphandler.GzipHandler(http.HandlerFunc(echoServer)))
	r.Handle("/ws-pinger", gziphandler.GzipHandler(http.HandlerFunc(pinger)))
	go func() {
		if certfile == "" {
			return
		}
		err := http.ListenAndServeTLS(":"+strconv.FormatInt(int64(httpsPort), 10),
			certfile, certfile, r)
		log.Fatal(err)
	}()
	log.Fatal(http.ListenAndServe(":"+strconv.FormatInt(int64(httpPort), 10), r))
}

func root(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, `Endpoints on this server:
	/slow - responds slowly - accepts query params: chunk, delay, duration
	/slam - closes the connection without writing headers or body - accepts query param: duration
	/slam/headers - closes connection after writing headers - accepts query param: duration
	/slam/body - closes connection after writing 1/2 the body - accepts query param: duration, len
	/connections - list (GET) and create (POST) remote TCP connections
	/headers - respond with headers sent as text body
	/ws-echo - a websocket connection which echoes lines in response
	/ws-pinger - a websocket connection which pings every 10s - accepts query param: delay
	/gs-echo - a go websocket connection which echoes lines in response
	/gs-pinger - a go websocket connection which pings every 10s - accepts query param: delay
The /gs-echo and /gs-pinger endpoints use golang.org/x/net/websocket which does
not use data framing as defined in RFC6455.
	`)
}

// slam closes the connection without writing anything.
func slam(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	t := timeQueryParam(r.Form, "duration", time.Duration(0))
	time.Sleep(t)
	panic("slam!")
}

// headerSlam writes some headers and then closes the connection before writing body.
func headerSlam(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	t := timeQueryParam(r.Form, "duration", time.Duration(0))
	time.Sleep(t)
	w.Header().Add("Content-Type", "text")
	w.Header().Add("Content-Length", "1024")
	w.WriteHeader(200)
}

// bodySlam writes headers and then closes the connection before completely writing body.
func bodySlam(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	t := timeQueryParam(r.Form, "duration", time.Duration(0))
	l := r.Form.Get("len")
	ll, err := strconv.Atoi(l)
	if err != nil {
		if l != "" {
			log.Print(err)
		}
		ll = 512
	}

	w.Header().Add("Content-Type", "text")
	w.Header().Add("Content-Length", strconv.Itoa(ll*2))
	w.WriteHeader(200)
	time.Sleep(t)
	f, err := os.Open("/usr/share/dict/words")
	if err != nil {
		log.Print("couldn't open /usr/share/dict/words")
		return
	}
	defer f.Close()
	io.Copy(w, io.LimitReader(f, int64(ll)))
}

func headers(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text")
	for i := range r.Header {
		for _, v := range r.Header[i] {
			io.WriteString(w, i)
			io.WriteString(w, ": ")
			io.WriteString(w, v)
			io.WriteString(w, "\n")
		}
	}
}

func connections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		connectionsGet(w, r)
	case http.MethodPost:
		connectionsPost(w, r)
	case http.MethodDelete:
		connectionsDelete(w, r)
	}
}

// TODO: support more content types.
func connectionsGet(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	long := strings.ToLower(r.FormValue("long"))
	for i := range conns {
		if i != 0 {
			fmt.Fprintln(w)
		}
		c := conns[i]
		fmt.Fprintf(w, "%d reconnects:%d totalRR:%d %s %s\n", i,
			c.reconnects, c.totalRR, c.RemoteAddr(), c.LocalAddr())
		switch {
		case long == "true" || len(c.last_read) < 80:
			fmt.Fprintf(w, "last read: %s\n", c.last_read)
		default:
			fmt.Fprintf(w, "last read: %s\n", c.last_read[0:80])
		}
		if c.err != nil {
			fmt.Fprintf(w, "err: %v", c.err)
			c.err = nil
		}
	}
}

func connectionsPost(w http.ResponseWriter, r *http.Request) {
	buf := make([]byte, 1024)
	n, err := r.Body.Read(buf)
	if err != nil && err != io.EOF {
		log.Printf("error reading request body %#v: %s", r, err)
	}
	addConnection(string(buf[0:n]))
	w.WriteHeader(http.StatusAccepted)
}

func connectionsDelete(w http.ResponseWriter, r *http.Request) {
	buf := make([]byte, 1024)
	n, err := r.Body.Read(buf)
	if err != nil && err != io.EOF {
		log.Printf("error reading request body %#v: %s", r, err)
	}
	i, err := strconv.Atoi(string(buf[0:n]))
	if err != nil {
		log.Printf("error parsing request body %#v: %s", r, err)
		http.Error(w, http.StatusText(http.StatusBadRequest),
			http.StatusBadRequest)
		return
	}
	rmConnection(i)
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
		http.Error(w, "could not stat /usr/share/dict/words", 500)
		return
	}
	src, dst := f, w
	sz := int(st.Size())
	dd := int(t / delay)
	chunk := 10
	if dd != 0 {
		chunk = sz / dd
	}
	if c, err := strconv.ParseInt(r.Form.Get("chunk"), 10, 64); err == nil {
		chunk = int(c)
	} else {
		log.Print("failed to parse chunk query param", r.Form.Get("chunk"))
	}
	log.Printf("/slow writing %d every %s for %s", chunk, delay, t)
	// TODO: consider calculating correct content-length and setting it
	if t == 5*time.Minute {
		w.Header().Set("content-length", strconv.Itoa(sz))
	}
	buf := make([]byte, chunk)
	start := time.Now()
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
			if time.Since(start) > t {
				break
			}
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

var upgrader = websocket.Upgrader{} // use default options
// Echo the data received on the WebSocket.
func echoServer(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)
		err = c.WriteMessage(mt, message)
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

func pinger(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	delay := timeQueryParam(r.Form, "delay", 10*time.Second)
	n := 0
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("pinger upgrade:", err)
		return
	}
	defer c.Close()
	for {
		// TODO: use c.PingHandler()
		n++
		err = c.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("%d\n", n)))
		if err != nil {
			if !errors.Is(err, syscall.EPIPE) && err != io.ErrClosedPipe {
				log.Printf("pinger write error: %s", err)
			}
			return
		}
		time.Sleep(delay)
	}
}

// Echo the data received on the WebSocket.
func echoServerXNWS(ws *xnws.Conn) {
	io.Copy(ws, ws)
}

func pingerXNWS(ws *xnws.Conn) {
	r := ws.Request()
	r.ParseForm()
	delay := timeQueryParam(r.Form, "delay", 10*time.Second)
	buf := make([]byte, 1500)
	n := 0
	for {
		ws.SetReadDeadline(time.Now().Add(1 * time.Second))
		br, err := ws.Read(buf)
		if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
			if errors.Is(err, io.EOF) {
				return
			}
			log.Printf("pinger read error: %s %T", err, err)
			return
		}
		if br > 0 {
			log.Printf("pinger read: %s", buf[:br])
		}
		time.Sleep(delay)
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

// Connection is a net.Conn wrapped for our purpose
type Connection struct {
	net.Conn
	err        error
	last_read  string
	addr       string
	delay      time.Duration
	reconnects int
	totalRR    int // total request responses.
	payload    string
}

func doinitconns(initconns string) {
	points := strings.Split(initconns, ",")
	for i := range points {
		addConnection(points[i])
	}
}

func addConnection(conndef string) error {
	addr, after, found := strings.Cut(conndef, "_")
	delay := time.Minute
	payload := ""
	if found {
		ds, ps, _ := strings.Cut(after, "_")
		d, err := time.ParseDuration(ds)
		if err != nil {
			log.Print("could not parse ", conndef, err)
			return err
		}
		delay = d
		payload = ps
	}
	c, err := net.Dial("tcp", addr)
	if err != nil {
		log.Print("error connecting to", addr, err)
		return err
	}
	// Do naive \r\n replacement. Sadly, no support for a literal.
	payload = strings.ReplaceAll(payload, "\\n", "\n")
	payload = strings.ReplaceAll(payload, "\\r", "\r")
	conn := &Connection{
		Conn:    c,
		addr:    addr,
		delay:   delay,
		payload: payload,
	}
	i := len(conns)
	conns = append(conns, conn)
	log.Printf("parsed connection %#v", conn)
	go connloop(i, conn)
	return nil
}

func rmConnection(i int) error {
	if i >= len(conns) {
		return errors.ErrUnsupported
	}
	err := conns[i].Close()
	if err != nil {
		log.Printf("error closing conn %d %s", i, err)
		// Intentionally not returning here because we must
	}
	// Use nil as sentinel that it has been removed.
	conns[i].Conn = nil
	conns = slices.Delete(conns, i, i+1)
	return nil
}

func replaceConnection(i int) *Connection {
	err := conns[i].Close()
	if err != nil {
		log.Printf("error closing conn %s", err)
	}
	c, err := net.Dial("tcp", conns[i].addr)
	if err != nil {
		log.Print("error connecting to", conns[i].addr, err)
	}
	conns[i] = &Connection{
		Conn:       c,
		err:        err,
		addr:       conns[i].addr,
		delay:      conns[i].delay,
		reconnects: conns[i].reconnects + 1,
		payload:    conns[i].payload,
	}
	return conns[i]
}

func connloop(i int, c *Connection) {
	buffer := make([]byte, 1024)
	for {
		if c.Conn == nil {
			log.Print("ending old loop", i, "no connection")
			return
		}
		n, err := fmt.Fprintf(c, c.payload)
		if err != nil {
			log.Printf("error writing to %v: %v", c, err)
			c.err = err
			c = replaceConnection(i)
			continue
		}
		if n == 0 {
			log.Printf("error 2 writing to %v: write returned 0", c)
			c.err =
				fmt.Errorf("error 2 writing to %v: write returned 0", c)
		}
		n, err = c.Read(buffer)
		if err != nil {
			log.Printf("error reading from %v", err)
			c.err = fmt.Errorf("error reading from %v: Read returned 0", c)
			c = replaceConnection(i)
			continue
		}
		c.last_read = string(buffer[0:n])
		c.totalRR += 1
		time.Sleep(c.delay)
	}
}
