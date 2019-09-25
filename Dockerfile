# Build stage
FROM golang:1-alpine AS builder

WORKDIR /go/src/github.com/splitio/split-synchronizer

COPY . .

RUN apk update && \
    apk add --no-cache git

RUN wget https://github.com/golang/dep/releases/download/v0.5.4/dep-linux-amd64
RUN chmod +x dep-linux-amd64
RUN ./dep-linux-amd64 ensure

RUN go build -o split-sync

# Runner stage
FROM alpine:latest AS runner

COPY entrypoint.sh .

COPY --from=builder /go/src/github.com/splitio/split-synchronizer/split-sync /usr/bin/

EXPOSE 3000 3010

ENTRYPOINT ["sh", "entrypoint.sh"]
