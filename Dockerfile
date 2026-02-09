FROM golang:1.25.5 AS builder

WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    make \
    jq \
 && rm -rf /var/lib/apt/lists/*

COPY . .

RUN make install

FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    bash \
    jq \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /go/bin/evmd /usr/local/bin/evmd
COPY ./local_node.sh /app/local_node.sh

RUN chmod +x /app/local_node.sh

EXPOSE 26657 9090 1317 8545 8546

CMD ["bash", "-lc", "./local_node.sh -y --no-install"]
