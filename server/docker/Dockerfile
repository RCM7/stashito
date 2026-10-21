FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder
ARG TARGETOS TARGETARCH TARGETVARIANT
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=$([ "$TARGETARCH" = arm ] && echo "${TARGETVARIANT#v}") go build -o /stashito-server .

FROM alpine:3.21 AS runtime
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /app
COPY --from=builder /stashito-server .
EXPOSE 8080
CMD ["./stashito-server"]
