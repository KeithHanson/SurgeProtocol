FROM golang:1.23-alpine

WORKDIR /app

COPY . .

RUN go mod tidy
RUN go build -o surge-protocol-go-client .

ENTRYPOINT ["./surge-protocol-go-client"]