# Build stage
FROM golang:1.17.1-alpine3.14 AS builder

WORKDIR /go/src/github.com/splitio/split-synchronizer

COPY . .

RUN go build -o split-sync

# Runner stage
FROM alpine:3.14 AS runner

RUN addgroup -g 1000 -S 'split-synchronizer'
RUN adduser \
    --disabled-password \
    --gecos '' \
    --ingroup 'split-synchronizer' \
    --no-create-home \
    --system \
    --uid 1000 \
    'split-synchronizer'

COPY entrypoint.sh .

COPY --from=builder /go/src/github.com/splitio/split-synchronizer/split-sync /usr/bin/

EXPOSE 3000 3010

USER 'split-synchronizer'

ENTRYPOINT ["sh", "entrypoint.sh"]
