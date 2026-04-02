FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o /bin/service ./main.go

FROM alpine:3.20
RUN adduser -D appuser
USER appuser
COPY --from=builder /bin/service /bin/service
EXPOSE 8080
ENTRYPOINT ["/bin/service"]
