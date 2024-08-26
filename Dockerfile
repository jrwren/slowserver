# syntax=docker/dockerfile:1
FROM golang:1.23.0 as build

WORKDIR /usr/src/app

COPY . slowserver
RUN cd slowserver && go install .

FROM ubuntu:24.04

RUN apt-get -y update && apt install -y --no-install-recommends wamerican

COPY --from=build /go/bin/slowserver /
# --build-arg buildTime=$(printf '%(%s)T') or $(date +%s)
ARG buildTime
ENV buildTime=$buildTime

CMD ["/slowserver"]
