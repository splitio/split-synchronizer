FROM golang:1.8

WORKDIR /go/src/github.com/splitio/go-agent

COPY . .

RUN go build -o split-sync

RUN cp split-sync /usr/bin/split-sync

EXPOSE 3000 3010

ENTRYPOINT ["sh", "entrypoint.sh"]
