FROM golang:1.25-alpine AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o webhook ./cmd/webhook/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /workspace/webhook .

EXPOSE 8443

ENTRYPOINT ["./webhook"]
