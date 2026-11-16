FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# Static binary on a minimal, non-root image (includes CA certificates for
# the outbound HTTPS calls to Gemini and Drive).
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
