FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY backend/go.mod ./
COPY backend/ ./
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /task-tracker ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /task-tracker /app/task-tracker
COPY docs/openapi.yaml /app/docs/openapi.yaml
USER app
EXPOSE 8080
CMD ["/app/task-tracker"]