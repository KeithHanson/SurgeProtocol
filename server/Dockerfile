FROM golang:1.23-alpine

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o surge-protocol-server .

EXPOSE 8080

ENTRYPOINT ["sh", "./run.sh"]
