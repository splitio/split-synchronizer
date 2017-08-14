FROM golang:1.8

WORKDIR /go/src/github.com/splitio/go-agent

COPY . .

RUN go build

EXPOSE 3000 3010

ENTRYPOINT ["./go-agent"]
