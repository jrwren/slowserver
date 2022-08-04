# slowserver

Slowserver is a simple web app with an intentionally slow responding endpoint
and a websocket echo and websocket pinger endpoint.

This can be useful for testing HTTP clients and proxies.

Also included is wsocat, a command line websocket client.

## Running

Use go install to install slowserver and wsocat.

```sh
$ go install github.com/jrwren/slowserver/...
```

In one shell run the server:

```sh
$ slowserver
```

Then run wsocat to connect to it:

```sh
$ wsocat ws://localhost:8080/ws-pinger
```
