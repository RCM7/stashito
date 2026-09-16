FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /stashito-server .

FROM alpine:3.21 AS runtime
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /stashito-server .
EXPOSE 8080
CMD ["./stashito-server"]
