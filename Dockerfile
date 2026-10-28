# syntax=docker/dockerfile:1
FROM golang:latest AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /nehome ./cmd/nehome

# distroless : certificats CA et utilisateur non root, ni shell ni gestionnaire de paquets.
FROM gcr.io/distroless/static:nonroot
COPY --from=build /nehome /nehome
USER nonroot:nonroot
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/nehome", "healthcheck"]
ENTRYPOINT ["/nehome"]
CMD ["serve"]
