# syntax=docker/dockerfile:1

# Build stage: compile the server and migrate binaries.
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./server/cmd/server && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/stonewrit ./server/cmd/stonewrit

# Runtime stage: a minimal, non-root distroless image.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/stonewrit /app/stonewrit

EXPOSE 3002
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
