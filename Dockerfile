# syntax=docker/dockerfile:1
# Compilation croisée : le binaire Go est construit sur la plateforme du builder pour chaque cible.
FROM --platform=$BUILDPLATFORM golang:latest AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /nehome ./cmd/nehome

# distroless : certificats CA et utilisateur non root, ni shell ni gestionnaire de paquets.
FROM gcr.io/distroless/static:nonroot
COPY --from=build /nehome /nehome
USER nonroot:nonroot
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/nehome", "healthcheck"]
ENTRYPOINT ["/nehome"]
CMD ["serve"]
