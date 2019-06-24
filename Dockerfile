FROM  golang:1.12.6-stretch

WORKDIR /go/src/github.com/splitio/split-synchronizer

COPY . .

RUN wget https://github.com/golang/dep/releases/download/v0.3.2/dep-linux-amd64
RUN chmod +x dep-linux-amd64
RUN ./dep-linux-amd64 ensure

RUN go build -o split-sync

RUN cp split-sync /usr/bin/split-sync

EXPOSE 3000 3010

ENTRYPOINT ["sh", "entrypoint.sh"]
