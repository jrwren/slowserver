package res

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync/atomic"

	"golang.org/x/exp/slices"
)

// https://koraygocmen.medium.com/custom-dns-resolver-for-the-default-http-client-in-go-a1420db38a5d
// and https://github.com/benburkert/dns/blob/d356cf78cdfc/init/init.go
type Override struct {
	Addrs []string
	n     int32
	H     string
	seen  []string
}

// Dial is a terribly poorly written function which needs much love.
func (as *Override) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aoverride SHP error:%v\n", err)
		os.Exit(1)
	}
	if host != as.H {
		c, err := net.Dial(network, address)
		raddr := c.RemoteAddr()
		if !slices.Contains(as.seen, raddr.String()) {
			as.seen = append(as.seen, raddr.String())
			fmt.Fprintf(os.Stderr, "NO aoverride dial(%s,%s) for %s host=%s", network, address, as.H, host)
			fmt.Fprintf(os.Stderr, "!! dialed %s\n", raddr)
		}
		return c, err
	}
	a := as.Addrs[int(as.n)%len(as.Addrs)] + ":" + port
	atomic.AddInt32(&as.n, 1)
	//fmt.Fprintf(os.Stderr, "aoverride dial %s %s using %s\n", network, address, a)

	// I want to do this, but nettrace is internal :(
	// trace, _ := ctx.Value(nettrace.TraceKey{}).(*nettrace.Trace)
	// trace.DNSDone(a, )
	// So instead???

	c, err := net.Dial(network, a)
	if err != nil {
		log.Println("aoverride dial error dialing ", network, " ", a, ":", err)
	}
	return c, err
}
