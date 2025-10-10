# slowserver

Slowserver is a simple web app with an intentionally slow responding endpoint
and a websocket echo and websocket pinger endpoint.

This can be useful for testing HTTP clients and proxies.

Also included is wsocat, a command line websocket client.

## Running

Use go install to install slowserver and wsocat.

```sh
go install github.com/jrwren/slowserver/...
```

In one shell run the server:

```sh
slowserver
```

Then run wsocat to connect to it:

```sh
wsocat ws://localhost:8080/ws-pinger
```

## Running via Docker

```
docker run `ghcr.io/jrwren/slowserver:latest`
```

![Docker Build & Push](https://github.com/jrwren/slowserver/actions/workflows/docker-image.yml/badge.svg)

## Docker Image

The latest image is published to GitHub Container Registry:

`ghcr.io/jrwren/slowserver:latest`

[View on GHCR](https://github.com/users/jrwren/packages/container/slowserver)
