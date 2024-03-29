FROM golang:1.21.4 as build

WORKDIR /usr/src/app

COPY . slowserver
#RUN git clone https://github.com/jrwren/slowserver.git &&\
RUN   cd slowserver && go install .

FROM ghcr.io/jrwren/cloud-apps-baser

RUN openssl req \
    -nodes -newkey rsa:2048 \
    -keyout slowserver.key \
    -out slowserver.csr \
    -subj "/C=US/ST=MI/L=AA/O=XMTP/OU=jrwren/CN=slowserver.example.com" \
  && \
  openssl x509 -req -days 3650 \
    -in slowserver.csr \
    -signkey slowserver.key \
    -out slowserver.crt  && \
  cat slowserver.crt slowserver.key > slowserver.pem && \
  cp slowserver.pem /certificates
RUN apt update && apt install --no-install-recommends wamerican
ADD run.sh /run.sh
ADD env.sh /env.sh
COPY --from=build /go/bin/slowserver /app
# --build-arg buildTime=$(printf '%(%s)T') or $(date +%s)
ARG buildTime
ENV buildTime=$buildTime
