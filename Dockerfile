FROM golang:1.25-alpine AS source
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM source AS test
CMD ["sh", "-c", "test -z \"$(gofmt -l cmd internal migrations)\" && go build ./... && go vet ./... && go test -count=1 -v ./..."]

FROM source AS build
RUN CGO_ENABLED=0 go build -trimpath -o /out/taskmanager ./cmd/taskmanager

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 taskmanager
WORKDIR /app
COPY --from=build /out/taskmanager ./taskmanager
USER taskmanager
EXPOSE 8080
ENTRYPOINT ["/app/taskmanager"]
CMD ["serve"]
