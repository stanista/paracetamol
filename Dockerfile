FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY main.go ./

RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o paracetamol .

FROM alpine:latest

RUN apk add --no-cache docker-cli

COPY --from=builder /app/paracetamol /usr/local/bin/paracetamol

ENTRYPOINT ["paracetamol"]
